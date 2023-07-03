package server

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"geopki/pkg/crypto"
	"geopki/pkg/database"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

func (env *EndpointHandlerEnv) postInsert(c *gin.Context) {
	if !env.receivedValidInsertionKey(c) {
		return
	}

	// check the content type request header
	contentTypeHeaders, ok := c.Request.Header["Content-Type"]
	if ok {
		// if the content type header is set, make sure it is exactly 'application/json'
		if len(contentTypeHeaders) > 1 || contentTypeHeaders[0] != "application/json" {

			// display an error to the user
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf(
					"Content-Type:%s is not supported. Don't set the header or use 'application/json'.",
					contentTypeHeaders[0],
				),
			})

			return
		}
	}

	// by default the hashes are updated, can be turned off for partial insertions,
	// especially the initial insertion where hashes are computed over and over again otherwise
	isPartialUpdate := c.DefaultQuery("is-partial", "none") != "none"

	// read request body
	zr, err := gzip.NewReader(c.Request.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating gzip reader failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "creating gzip reader, check the server logs",
		})
		return
	}

	body, err := io.ReadAll(zr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading gzipped request body failed: %v\n", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	if err := zr.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "closing gzip reader failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "closing gzip reader, check the server logs",
		})
		return
	}

	var certificates []*crypto.GeoCertificate
	err = json.Unmarshal(body, &certificates)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf(
				"supplied invalid certificates, %v",
				err,
			),
		})
		return
	}

	for _, certificate := range certificates {
		marshaledCert, err := json.Marshal(certificate)
		if err != nil {
			fmt.Fprintf(os.Stderr, "marshaling certificate failed: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "marshaling certificates failed, check the server logs",
			})
			return
		}
		certificate.MarshaledCert = marshaledCert
	}

	didLock := env.UpdateLock.TryLock()
	if !didLock {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "update is already in progress, try again later",
		})
		return
	}

	// unlock after returning
	defer env.UpdateLock.Unlock()

	tx, err := env.DbPool.BeginTx(c.Request.Context(), pgx.TxOptions{
		IsoLevel: pgx.Serializable,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "starting transaction failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "starting transaction failed, check the server logs",
		})
		return
	}

	defer tx.Rollback(c.Request.Context())

	err = database.AddNewCertificates(
		certificates,
		F_GROW,
		tx,
		// update hashes if it is *not* a partial update
		!isPartialUpdate,
		c.Request.Context(),
	)

	if err != nil {
		fmt.Fprintf(os.Stderr, "failed updating SMT: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed updating SMT, check the server logs",
		})
		return
	}

	var smh *crypto.SignedMapHead

	if isPartialUpdate {
		// if set to false, set to true, noop if already true
		_, err := database.UpdateState(DATABASE_STATE_KEY_DIRTY, "false", "true", tx, c.Request.Context())
		if err != nil {
			fmt.Fprintf(os.Stderr, "updating state '%s' failed: %v\n", DATABASE_STATE_KEY_DIRTY, err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "updating internal state failed, check the server logs",
			})
			return
		}
	} else {
		smh, err = CreateNewSMH(tx, env.PrivateKey, c.Request.Context())
		if err != nil {
			fmt.Fprintf(os.Stderr, "updating SMH failed: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "updating SMH failed, check the server logs",
			})
			return
		}
	}

	// before transaction is commited, acquire lock on shared data
	env.SharedDataLock.Lock()
	defer env.SharedDataLock.Unlock()

	err = tx.Commit(c.Request.Context())
	if err != nil {
		fmt.Fprintf(os.Stderr, "commiting transaction failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "commiting transaction failed, check the server logs",
		})
		return
	}

	if isPartialUpdate {
		// update was persisted and we acquired a lock, update the shared state
		env.IsDirty = true
	} else {
		// update sch if transaction committed
		_, err := env.updateSCH(smh, c.Request.Context())
		if err != nil {
			fmt.Fprintf(os.Stderr, "updating SCH failed: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "updating SCH failed, check the server logs",
			})
			return
		}
	}

	c.Data(
		http.StatusOK,
		"application/json",
		[]byte("{\"success\":true}"),
	)
}

func (env *EndpointHandlerEnv) postDropIndices(c *gin.Context) {
	if !env.receivedValidInsertionKey(c) {
		return
	}

	didLock := env.UpdateLock.TryLock()
	if !didLock {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Update is already in progress, try again later",
		})
		return
	}

	// unlock after returning
	defer env.UpdateLock.Unlock()

	tx, err := env.DbPool.BeginTx(c.Request.Context(), pgx.TxOptions{
		IsoLevel: pgx.Serializable,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "starting transaction failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "starting transaction failed, check the server logs",
		})
		return
	}

	defer tx.Rollback(c.Request.Context())

	// drop indices for faster insertion
	_, err = tx.Exec(
		c.Request.Context(),
		// nodes table
		"DROP INDEX IF EXISTS bit_string_bit_idx;"+
			"DROP INDEX IF EXISTS bit_string_len;"+
			"DROP INDEX IF EXISTS bit_string_integer_idx;"+
			// certificates table
			"DROP INDEX IF EXISTS certificate_hash;"+
			"DROP INDEX IF EXISTS certificate_not_valid_after;",
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dropping indices failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "dropping indices failed",
		})
		return
	}

	// update persistent state to dirty
	_, err = database.UpdateState(DATABASE_STATE_KEY_DIRTY, "false", "true", tx, c.Request.Context())
	if err != nil {
		fmt.Fprintf(os.Stderr, "updating state '%s' failed: %v\n", DATABASE_STATE_KEY_DIRTY, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "updating internal state failed, check the server logs",
		})
		return
	}

	// before the data appears in the db, acquire a lock on the cached data
	// otherwise a reader might observe inconsistent data
	env.SharedDataLock.Lock()
	defer env.SharedDataLock.Unlock()

	err = tx.Commit(c.Request.Context())
	if err != nil {
		fmt.Fprintf(os.Stderr, "commiting transaction failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "commiting transaction failed, check the server logs",
		})
		return
	}

	// after sucessful commitment, update shared state
	// we already acquired the lock
	env.IsDirty = true

	c.Data(
		http.StatusOK,
		"application/json",
		[]byte("{\"success\":true}"),
	)
}

func (env *EndpointHandlerEnv) postFinishPartial(c *gin.Context) {
	if !env.receivedValidInsertionKey(c) {
		return
	}

	didLock := env.UpdateLock.TryLock()
	if !didLock {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Update is already in progress, try again later",
		})
		return
	}

	// unlock after returning
	defer env.UpdateLock.Unlock()

	tx, err := env.DbPool.BeginTx(c.Request.Context(), pgx.TxOptions{
		IsoLevel: pgx.Serializable,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "starting transaction failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "starting transaction failed, check the server logs",
		})
		return
	}

	defer tx.Rollback(c.Request.Context())

	// re-compute all hashes
	_, err = tx.Exec(c.Request.Context(), "SELECT compute_hashes()")
	if err != nil {
		fmt.Fprintf(os.Stderr, "updating hashes failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "updating hashes failed, check the server logs",
		})
		return
	}

	// re-create indices
	_, err = tx.Exec(
		c.Request.Context(),
		// nodes table
		"CREATE UNIQUE INDEX IF NOT EXISTS bit_string_bit_idx ON nodes USING btree (bit_string_51 ASC NULLS LAST, bit_string_15 ASC NULLS LAST);"+
			"CREATE INDEX IF NOT EXISTS bit_string_len ON nodes (LENGTH(bit_string_51), LENGTH(bit_string_15));"+
			"CREATE INDEX IF NOT EXISTS bit_string_integer_idx ON nodes USING btree (bit_string_51_int ASC NULLS LAST);"+
			"ALTER TABLE IF EXISTS nodes CLUSTER ON bit_string_integer_idx;"+
			"CLUSTER nodes USING bit_string_integer_idx;"+
			// certificates table
			"CREATE INDEX IF NOT EXISTS certificate_hash ON certificates USING hash (certificate_hash);"+
			"CREATE INDEX IF NOT EXISTS certificate_not_valid_after ON certificates USING btree (not_valid_after);"+
			"ALTER TABLE IF EXISTS certificates CLUSTER ON certificate_not_valid_after;"+
			"CLUSTER certificates USING certificate_not_valid_after;",
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating indices and constraints failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "creating indices and constraints failed",
		})
		return
	}

	smh, err := CreateNewSMH(tx, env.PrivateKey, c.Request.Context())
	if err != nil {
		fmt.Fprintf(os.Stderr, "updating SMH failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "updating SMH failed, check the server logs",
		})
		return
	}

	// update persistent state, no longer dirty
	_, err = database.UpdateState(DATABASE_STATE_KEY_DIRTY, "true", "false", tx, c.Request.Context())
	if err != nil {
		fmt.Fprintf(os.Stderr, "updating state '%s' failed: %v\n", DATABASE_STATE_KEY_DIRTY, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "updating internal state failed, check the server logs",
		})
		return
	}

	// before the data appears in the db, acquire a lock on the cached data
	// otherwise a reader might observe inconsistent data
	env.SharedDataLock.Lock()
	defer env.SharedDataLock.Unlock()

	err = tx.Commit(c.Request.Context())
	if err != nil {
		fmt.Fprintf(os.Stderr, "commiting transaction failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "commiting transaction failed, check the server logs",
		})
		return
	}

	// after sucessful commitment, update shared state
	// we already acquired the lock
	env.IsDirty = false

	// update sch if transaction committed
	sch, err := env.updateSCH(smh, c.Request.Context())
	if err != nil {
		fmt.Fprintf(os.Stderr, "updating SCH failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "updating SCH failed, check the server logs",
		})
		return
	}

	c.Data(
		http.StatusOK,
		"application/json",
		[]byte(fmt.Sprintf("{\"success\":true, \"new_tree_size\":%d}", sch.Size)),
	)
}
