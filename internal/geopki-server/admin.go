package server

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"geopki/pkg/crypto"
	"geopki/pkg/database"

	"github.com/jackc/pgx/v5"
	"github.com/valyala/fasthttp"
)

const (
	defaultAdminListLimit = 100
	maxAdminListLimit     = 500
)

// JSON representation of a single certificate row, returned by the admin listing endpoint.
type adminCertificateEntry struct {
	Hash          string          `json:"hash"`
	NotValidAfter time.Time       `json:"not_valid_after"`
	Expired       bool            `json:"expired"`
	Certificate   json.RawMessage `json:"certificate"`
}

// handler for the /v1/admin/certificates endpoint, lists all certificates, paginated
func (env *EndpointHandlerEnv) getAdminCertificates(ctx *fasthttp.RequestCtx) {
	if !env.receivedValidInsertionKey(ctx) {
		return
	}

	limit := defaultAdminListLimit
	if raw := ctx.QueryArgs().Peek("limit"); raw != nil {
		parsed, err := strconv.Atoi(string(raw))
		if err != nil || parsed <= 0 {
			errorHandler(ctx, fasthttp.StatusBadRequest, "invalid 'limit' parameter")
			return
		}
		limit = parsed
	}
	if limit > maxAdminListLimit {
		limit = maxAdminListLimit
	}

	offset := 0
	if raw := ctx.QueryArgs().Peek("offset"); raw != nil {
		parsed, err := strconv.Atoi(string(raw))
		if err != nil || parsed < 0 {
			errorHandler(ctx, fasthttp.StatusBadRequest, "invalid 'offset' parameter")
			return
		}
		offset = parsed
	}

	entries, total, err := database.ListCertificates(env.DbPool, limit, offset, ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "listing certificates failed: %v\n", err)
		errorHandler(ctx, fasthttp.StatusInternalServerError, "listing certificates failed")
		return
	}

	now := time.Now()

	response := struct {
		Total        int                     `json:"total"`
		Limit        int                     `json:"limit"`
		Offset       int                     `json:"offset"`
		Certificates []adminCertificateEntry `json:"certificates"`
	}{
		Total:        total,
		Limit:        limit,
		Offset:       offset,
		Certificates: make([]adminCertificateEntry, len(entries)),
	}

	for i, entry := range entries {
		response.Certificates[i] = adminCertificateEntry{
			Hash:          base64.RawURLEncoding.EncodeToString(entry.Hash[:]),
			NotValidAfter: entry.NotValidAfter,
			Expired:       !entry.NotValidAfter.After(now),
			Certificate:   json.RawMessage(entry.Certificate),
		}
	}

	body, err := json.Marshal(response)
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshaling admin certificate list failed: %v\n", err)
		errorHandler(ctx, fasthttp.StatusInternalServerError, "marshaling response failed")
		return
	}

	ctx.SetContentType("application/json")
	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBody(body)
}

// handler for the /v1/admin/expire endpoint. back-dates 'not_valid_after' for
// the given certificate hashes to now, so they are swept out of the tree the
// next time '/v1/release' runs -- it does not remove them immediately.
func (env *EndpointHandlerEnv) postAdminExpire(ctx *fasthttp.RequestCtx) {
	if !env.receivedValidInsertionKey(ctx) {
		return
	}

	var request struct {
		Hashes []string `json:"hashes"`
	}

	if err := json.Unmarshal(ctx.Request.Body(), &request); err != nil {
		errorHandler(ctx, fasthttp.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	if len(request.Hashes) == 0 {
		errorHandler(ctx, fasthttp.StatusBadRequest, "no certificate hashes supplied")
		return
	}

	hashes := make([]crypto.SHA256Hash, len(request.Hashes))
	for i, encoded := range request.Hashes {
		decoded, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil || len(decoded) != 32 {
			errorHandler(ctx, fasthttp.StatusBadRequest, fmt.Sprintf("invalid certificate hash: %s", encoded))
			return
		}
		hashes[i] = crypto.BytesToHash(decoded)
	}

	didLock := env.UpdateLock.TryLock()
	if !didLock {
		errorHandler(ctx, fasthttp.StatusBadRequest, "update is already in progress, try again later")
		return
	}
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

	affected, err := database.ExpireCertificates(hashes, time.Now(), tx, ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "expiring certificates failed: %v\n", err)
		errorHandler(ctx, fasthttp.StatusInternalServerError, "expiring certificates failed")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "commiting transaction failed: %v\n", err)
		errorHandler(ctx, fasthttp.StatusInternalServerError, "commiting transaction failed")
		return
	}

	body, _ := json.Marshal(struct {
		Success bool  `json:"success"`
		Expired int64 `json:"expired"`
	}{true, affected})

	ctx.SetContentType("application/json")
	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBody(body)
}
