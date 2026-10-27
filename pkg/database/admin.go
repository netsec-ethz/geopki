package database

import (
	"context"
	"fmt"
	"time"

	"geopki/pkg/crypto"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CertificateListEntry is a single row of the 'certificates' table, exposed
// for administrative listing purposes.
type CertificateListEntry struct {
	Hash          crypto.SHA256Hash
	Certificate   []byte
	NotValidAfter time.Time
}

// ListCertificates returns a page of all certificates plus the total number
// of certificates in the table. Ordered by 'not_valid_after', which matches
// the table's physical clustering (see database/tables/certificates.sql), so
// this is a cheap scan rather than a sort.
func ListCertificates(
	pool *pgxpool.Pool,
	limit, offset int,
	ctx context.Context,
) ([]CertificateListEntry, int, error) {

	var total int
	err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM certificates").Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed counting certificates: %v", err)
	}

	rows, err := pool.Query(
		ctx,
		"SELECT certificate_hash, certificate, not_valid_after FROM certificates "+
			"ORDER BY not_valid_after ASC, certificate_hash ASC LIMIT $1 OFFSET $2",
		limit,
		offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("failed listing certificates: %v", err)
	}
	defer rows.Close()

	entries := make([]CertificateListEntry, 0, limit)
	for rows.Next() {
		var dbHash, dbCertificate []byte
		var notValidAfter time.Time

		if err := rows.Scan(&dbHash, &dbCertificate, &notValidAfter); err != nil {
			return nil, 0, err
		}

		entries = append(entries, CertificateListEntry{
			Hash:          crypto.BytesToHash(dbHash),
			Certificate:   dbCertificate,
			NotValidAfter: notValidAfter,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return entries, total, nil
}

// ExpireCertificates immediately back-dates 'not_valid_after' to 't' for the
// given certificate hashes. This does *not* remove the certificates from the
// SMT or the 'certificates' table directly -- like any other expired
// certificate they are only swept out the next time '/v1/release' runs (see
// RemoveExpiredCertificates). Returns the number of rows actually updated.
func ExpireCertificates(
	hashes []crypto.SHA256Hash,
	t time.Time,
	transaction pgx.Tx,
	ctx context.Context,
) (int64, error) {
	if len(hashes) == 0 {
		return 0, nil
	}

	hashBytes := make([][]byte, len(hashes))
	for i, h := range hashes {
		hashBytes[i] = h[:]
	}

	tag, err := transaction.Exec(
		ctx,
		"UPDATE certificates SET not_valid_after = $1 "+
			"WHERE certificate_hash = ANY($2) AND not_valid_after > $1",
		t,
		hashBytes,
	)
	if err != nil {
		return 0, fmt.Errorf("failed expiring certificates: %v", err)
	}

	return tag.RowsAffected(), nil
}
