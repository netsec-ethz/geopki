package server

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

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

	// read request body
	zr, err := gzip.NewReader(c.Request.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating gzip reader failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "creating gzip reader",
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
			"error": "closing gzip reader",
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
				"error": "marshaling certificates failed",
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
			"error": "starting transaction failed",
		})
		return
	}

	defer tx.Rollback(c.Request.Context())

	err = database.AddNewCertificates(
		certificates,
		F_GROW,
		tx,
		c.Request.Context(),
	)

	if err != nil {
		fmt.Fprintf(os.Stderr, "failed updating SMT: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed updating SMT",
		})
		return
	}

	err = tx.Commit(c.Request.Context())
	if err != nil {
		fmt.Fprintf(os.Stderr, "commiting transaction failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "commiting transaction failed",
		})
		return
	}

	c.Data(
		http.StatusOK,
		"application/json",
		[]byte("{\"success\":true}"),
	)
}

func (env *EndpointHandlerEnv) postRelaseNewVersion(c *gin.Context) {
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
			"error": "starting transaction failed",
		})
		return
	}

	defer tx.Rollback(c.Request.Context())

	t := time.Now()

	fmt.Printf("New release was initiated at %s.\n", t.Format("2006-01-02 15:04:05-07"))

	start := time.Now()
	err = database.RemoveExpiredCertificates(t, tx, c.Request.Context())
	if err != nil {
		fmt.Fprintf(os.Stderr, "removing expired certificates failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "removing expired certificates failed",
		})
		return
	}

	fmt.Printf("Removal of expired certificates took %f minutes.\n", time.Since(start).Minutes())

	// drop indices on 'nodes' table
	start = time.Now()
	_, err = tx.Exec(
		c.Request.Context(),
		// nodes table
		"DROP INDEX IF EXISTS bit_string_bit_idx;"+
			"DROP INDEX IF EXISTS bit_string_len;"+
			"DROP INDEX IF EXISTS bit_string_integer_idx;",
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dropping indices failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "dropping indices failed",
		})
		return
	}

	fmt.Printf("Dropping indices took %f minutes.\n", time.Since(start).Minutes())

	// create indices with the same names on 'nodes_next' and cluster the data accordingly
	start = time.Now()
	_, err = tx.Exec(
		c.Request.Context(),
		"CREATE UNIQUE INDEX IF NOT EXISTS bit_string_bit_idx ON nodes_next USING btree (bit_string_51 ASC NULLS LAST, bit_string_15 ASC NULLS LAST);"+
			"CREATE INDEX IF NOT EXISTS bit_string_len ON nodes_next (LENGTH(bit_string_51), LENGTH(bit_string_15));"+
			"CREATE INDEX IF NOT EXISTS bit_string_integer_idx ON nodes_next USING btree (bit_string_51_int ASC NULLS LAST);"+
			"ALTER TABLE IF EXISTS nodes_next CLUSTER ON bit_string_integer_idx;"+
			"CLUSTER nodes_next USING bit_string_integer_idx;",
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating indices failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "creating indices failed",
		})
		return
	}

	fmt.Printf("Rebuilding indices took %f minutes.\n", time.Since(start).Minutes())

	// swap nodes with nodes_next
	start = time.Now()
	_, err = tx.Exec(
		c.Request.Context(),
		"ALTER TABLE nodes RENAME TO nodes_old;"+
			"ALTER TABLE nodes_next RENAME TO nodes;"+
			"ALTER TABLE nodes_old RENAME TO nodes_next;",
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "swapping tables failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "swapping tables failed",
		})
		return
	}

	fmt.Printf("Swapping tables took %f minutes.\n", time.Since(start).Minutes())

	// 'nodes_next' contains stale data, truncate and replace with new data from 'nodes' (not the generated columns though!)
	start = time.Now()
	_, err = tx.Exec(
		c.Request.Context(),
		"TRUNCATE nodes_next;"+
			"INSERT INTO nodes_next SELECT bit_string_51,bit_string_15,xy_left_child_hash,xy_right_child_hash,z_left_child_hash,z_right_child_hash,certificate_hashes FROM nodes;",
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "updating nodes_next failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "updating nodes_next failed",
		})
		return
	}

	fmt.Printf("Preparing table for new insertions took %f minutes.\n", time.Since(start).Minutes())

	// create new SMH based on the new 'nodes' table
	smh, err := CreateNewSMH(t, tx, env.PrivateKey, c.Request.Context())
	if err != nil {
		fmt.Fprintf(os.Stderr, "updating SMH failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "updating SMH failed",
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
			"error": "commiting transaction failed",
		})
		return
	}

	// update sch if transaction committed, we already acquired the lock
	// and can now create a new sch and update the shared data
	sch, err := env.updateSCH(smh, c.Request.Context())
	if err != nil {
		fmt.Fprintf(os.Stderr, "updating SCH failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "updating SCH failed",
		})
		return
	}

	c.Data(
		http.StatusOK,
		"application/json",
		[]byte(fmt.Sprintf("{\"success\":true, \"new_tree_size\":%d}", sch.Size)),
	)
}
