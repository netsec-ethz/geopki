package server

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"geopki/pkg/crypto"
	"geopki/pkg/database"

	"github.com/jackc/pgx/v5"
	"github.com/valyala/fasthttp"
)

func (env *EndpointHandlerEnv) postInsert(ctx *fasthttp.RequestCtx) {
	if !env.receivedValidInsertionKey(ctx) {
		return
	}

	// read request body
	zr, err := gzip.NewReader(ctx.RequestBodyStream())
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating gzip reader failed: %v\n", err)
		errorHandler(ctx, fasthttp.StatusInternalServerError, "creating gzip reader")
		return
	}

	body, err := io.ReadAll(zr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading gzipped request body failed: %v\n", err)
		errorHandler(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	if err := zr.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "closing gzip reader failed: %v\n", err)
		errorHandler(ctx, fasthttp.StatusInternalServerError, "closing gzip reader")
		return
	}

	var certificates []*crypto.GeoCertificate
	err = json.Unmarshal(body, &certificates)
	if err != nil {
		errorHandler(ctx, fasthttp.StatusBadRequest, fmt.Sprintf(
			"supplied invalid certificates, %v",
			err,
		))
		return
	}

	for _, certificate := range certificates {
		marshaledCert, err := json.Marshal(certificate)
		if err != nil {
			fmt.Fprintf(os.Stderr, "marshaling certificate failed: %v\n", err)
			errorHandler(ctx, fasthttp.StatusInternalServerError, "marshaling certificates failed")
			return
		}
		certificate.MarshaledCert = marshaledCert
	}

	didLock := env.UpdateLock.TryLock()
	if !didLock {
		errorHandler(ctx, fasthttp.StatusBadRequest, "update is already in progress, try again later")
		return
	}

	// unlock after returning
	defer env.UpdateLock.Unlock()

	tx, err := env.DbPool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.Serializable,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "starting transaction failed: %v\n", err)
		errorHandler(ctx, fasthttp.StatusInternalServerError, "starting transaction failed")
		return
	}

	defer tx.Rollback(ctx)

	err = database.AddNewCertificates(
		certificates,
		F_GROW,
		tx,
		ctx,
	)

	if err != nil {
		fmt.Fprintf(os.Stderr, "failed updating SMT: %v\n", err)
		errorHandler(ctx, fasthttp.StatusInternalServerError, "failed updating SMT")
		return
	}

	err = tx.Commit(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "commiting transaction failed: %v\n", err)
		errorHandler(ctx, fasthttp.StatusInternalServerError, "commiting transaction failed")
		return
	}

	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBody([]byte("{\"success\":true}"))
}

func (env *EndpointHandlerEnv) postRelaseNewVersion(ctx *fasthttp.RequestCtx) {
	if !env.receivedValidInsertionKey(ctx) {
		return
	}

	didLock := env.UpdateLock.TryLock()
	if !didLock {
		errorHandler(ctx, fasthttp.StatusBadRequest, "release is already in progress, try again later")
		return
	}

	// unlock after returning
	defer env.UpdateLock.Unlock()

	tx, err := env.DbPool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.Serializable,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "starting transaction failed: %v\n", err)
		errorHandler(ctx, fasthttp.StatusInternalServerError, "starting transaction failed")
		return
	}

	defer tx.Rollback(ctx)

	t := time.Now()

	fmt.Printf("New release was initiated at %s.\n", t.Format("2006-01-02 15:04:05-07"))

	start := time.Now()
	err = database.RemoveExpiredCertificates(t, tx, ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "removing expired certificates failed: %v\n", err)
		errorHandler(ctx, fasthttp.StatusInternalServerError, "removing expired certificates failed")
		return
	}

	fmt.Printf("Removal of expired certificates took %f minutes.\n", time.Since(start).Minutes())

	// drop indices on 'nodes' table
	start = time.Now()
	_, err = tx.Exec(
		ctx,
		// nodes table
		"DROP INDEX IF EXISTS bit_string_bit_idx;"+
			"DROP INDEX IF EXISTS bit_string_len;"+
			"DROP INDEX IF EXISTS bit_string_integer_idx;",
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dropping indices failed: %v\n", err)
		errorHandler(ctx, fasthttp.StatusInternalServerError, "dropping indices failed")
		return
	}

	fmt.Printf("Dropping indices took %f minutes.\n", time.Since(start).Minutes())

	// create indices with the same names on 'nodes_next' and cluster the data accordingly
	start = time.Now()
	_, err = tx.Exec(
		ctx,
		"CREATE UNIQUE INDEX IF NOT EXISTS bit_string_bit_idx ON nodes_next USING btree (bit_string_51 ASC NULLS LAST, bit_string_15 ASC NULLS LAST);"+
			"CREATE INDEX IF NOT EXISTS bit_string_len ON nodes_next (LENGTH(bit_string_51), LENGTH(bit_string_15));"+
			"CREATE INDEX IF NOT EXISTS bit_string_integer_idx ON nodes_next USING btree (bit_string_51_int ASC NULLS LAST);"+
			"ALTER TABLE IF EXISTS nodes_next CLUSTER ON bit_string_integer_idx;"+
			"CLUSTER nodes_next USING bit_string_integer_idx;",
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating indices failed: %v\n", err)
		errorHandler(ctx, fasthttp.StatusInternalServerError, "creating indices failed")
		return
	}

	fmt.Printf("Rebuilding indices took %f minutes.\n", time.Since(start).Minutes())

	// swap nodes with nodes_next
	start = time.Now()
	_, err = tx.Exec(
		ctx,
		"ALTER TABLE nodes RENAME TO nodes_old;"+
			"ALTER TABLE nodes_next RENAME TO nodes;"+
			"ALTER TABLE nodes_old RENAME TO nodes_next;",
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "swapping tables failed: %v\n", err)
		errorHandler(ctx, fasthttp.StatusInternalServerError, "swapping tables failed")
		return
	}

	fmt.Printf("Swapping tables took %f minutes.\n", time.Since(start).Minutes())

	// 'nodes_next' contains stale data, truncate and replace with new data from 'nodes' (not the generated columns though!)
	start = time.Now()
	_, err = tx.Exec(
		ctx,
		"TRUNCATE nodes_next;"+
			"INSERT INTO nodes_next(bit_string_51,bit_string_15,xy_left_child_hash,xy_right_child_hash,z_left_child_hash,z_right_child_hash,certificate_hashes) SELECT bit_string_51,bit_string_15,xy_left_child_hash,xy_right_child_hash,z_left_child_hash,z_right_child_hash,certificate_hashes FROM nodes;",
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "updating nodes_next failed: %v\n", err)
		errorHandler(ctx, fasthttp.StatusInternalServerError, "updating nodes_next failed")
		return
	}

	fmt.Printf("Preparing table for new insertions took %f minutes.\n", time.Since(start).Minutes())

	// create new SMH based on the new 'nodes' table
	smh, err := CreateNewSMH(t, tx, env.PrivateKey, ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "updating SMH failed: %v\n", err)
		errorHandler(ctx, fasthttp.StatusInternalServerError, "updating SMH failed")
		return
	}

	// before the data appears in the db, acquire a lock on the cached data
	// otherwise a reader might observe inconsistent data
	env.SharedDataLock.Lock()
	defer env.SharedDataLock.Unlock()

	err = tx.Commit(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "commiting transaction failed: %v\n", err)
		errorHandler(ctx, fasthttp.StatusInternalServerError, "commiting transaction failed")
		return
	}

	// update sch if transaction committed, we already acquired the lock
	// and can now create a new sch and update the shared data
	sch, err := env.updateSCH(smh, ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "updating SCH failed: %v\n", err)
		errorHandler(ctx, fasthttp.StatusInternalServerError, "updating SCH failed")
		return
	}

	fmt.Printf("Release took a total of %f minutes.\n", time.Since(t).Minutes())

	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBody([]byte(fmt.Sprintf("{\"success\":true, \"new_tree_size\":%d}", sch.Size)))
}
