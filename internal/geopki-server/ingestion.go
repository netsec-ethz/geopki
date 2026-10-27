package server

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"geopki/pkg/crypto"
	"geopki/pkg/database"
	"geopki/pkg/extension"
	_ "geopki/pkg/extension/wifi"

	"github.com/jackc/pgx/v5"
	"github.com/valyala/fasthttp"
)

// parses the body of an insertion request, a JSON array of certificates. Each
// certificate keeps the bytes it was submitted with: re-marshaling it would
// drop every field the GeoCertificate struct doesn't know, such as extensions,
// and change the hash the issuer computed. Certificates whose structure or
// known extensions are invalid are rejected.
func parseCertificates(body []byte) ([]*crypto.GeoCertificate, error) {
	var rawCertificates []json.RawMessage
	if err := json.Unmarshal(body, &rawCertificates); err != nil {
		return nil, err
	}

	certificates := make([]*crypto.GeoCertificate, 0, len(rawCertificates))
	for i, raw := range rawCertificates {
		certificate, err := crypto.UnmarshalGeoCertificate(raw)
		if err != nil {
			return nil, fmt.Errorf("certificate %d: %v", i, err)
		}
		if err := certificate.CheckStructure(); err != nil {
			return nil, err
		}
		if err := extension.Validate(certificate); err != nil {
			return nil, err
		}
		certificates = append(certificates, certificate)
	}
	return certificates, nil
}

func (env *EndpointHandlerEnv) postInsert(ctx *fasthttp.RequestCtx) {
	if !env.receivedValidInsertionKey(ctx) {
		return
	}

	// read request body
	body, err := ctx.Request.BodyGunzip()
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading gzipped request body failed: %v\n", err)
		errorHandler(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	certificates, err := parseCertificates(body)
	if err != nil {
		errorHandler(ctx, fasthttp.StatusBadRequest, fmt.Sprintf(
			"supplied invalid certificates, %v",
			err,
		))
		return
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
			"DROP INDEX IF EXISTS bit_string_integer_idx;"+
			"ALTER TABLE nodes_next DROP CONSTRAINT nodes_next_pkey;",
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dropping indices failed: %v\n", err)
		errorHandler(ctx, fasthttp.StatusInternalServerError, "dropping indices failed")
		return
	}

	fmt.Printf("Dropping indices took %f minutes.\n", time.Since(start).Minutes())

	// create indices with the same names on 'nodes_next' and cluster the data accordingly
	// also create primary key constraint on 'nodes'
	start = time.Now()
	_, err = tx.Exec(
		ctx,
		"CREATE UNIQUE INDEX IF NOT EXISTS bit_string_bit_idx ON nodes_next USING btree (bit_string_51 ASC NULLS LAST, bit_string_15 ASC NULLS LAST);"+
			"CREATE INDEX IF NOT EXISTS bit_string_len ON nodes_next (LENGTH(bit_string_51), LENGTH(bit_string_15));"+
			"CREATE INDEX IF NOT EXISTS bit_string_integer_idx ON nodes_next USING btree (bit_string_51_int ASC NULLS LAST);"+
			"ALTER TABLE IF EXISTS nodes_next CLUSTER ON bit_string_integer_idx;"+
			"CLUSTER nodes_next USING bit_string_integer_idx;"+
			"ALTER TABLE nodes ADD CONSTRAINT nodes_next_pkey PRIMARY KEY (bit_string_51, bit_string_15);",
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
