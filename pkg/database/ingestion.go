package database

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"geopki/pkg/bitstring"
	"geopki/pkg/crypto"
	"geopki/pkg/geometry"
	"sort"
	"strings"
	"time"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func bytesSliceToPostgresArray(bs [][]byte) string {
	encodedExpiredCertificateHashes := make([]string, len(bs))
	for i, b := range bs {
		encodedExpiredCertificateHashes[i] = fmt.Sprintf("E'\\\\x%s'::bytea", hex.EncodeToString(b))
	}

	expiredCertificateHashesArray := "ARRAY[" + strings.Join(encodedExpiredCertificateHashes, ",") + "]::bytea[]"

	return expiredCertificateHashesArray
}

// retrieves the set of all certificates in the database that expire before the given time
// according to https://www.rfc-editor.org/rfc/rfc5280#section-4.1.2.5, the values are inclusive
func ExpiredCertificateHashes(
	timestamp string,
	transaction pgx.Tx,
	ctx context.Context,
) ([]crypto.SHA256Hash, error) {
	rows, err := transaction.Query(
		ctx,
		"SELECT certificate_hash "+
			"FROM certificates "+
			"WHERE not_valid_after < '"+timestamp+"'",
	)
	if err != nil {
		return nil, fmt.Errorf("failed selecting expired certificates: %v", err)
	}

	defer rows.Close()

	// collect all certificate hashes
	certificateHashes := make([]crypto.SHA256Hash, 0)

	for rows.Next() {
		var dbCertificateHash []byte
		err := rows.Scan(&dbCertificateHash)

		if err != nil {
			return nil, err
		}

		certificateHashes = append(certificateHashes, dbCertificateHash)
	}

	err = rows.Err()
	if err != nil {
		return nil, err
	}

	return certificateHashes, nil
}

// removes the set of expired certificates and returns the list of bit strings that
// need updating
func RemoveExpiredCertificates(
	t time.Time,
	transaction pgx.Tx,
	ctx context.Context,
) (map[bitstring.RawBitStringPair]([]crypto.SHA256Hash), error) {

	timestamp := t.Format("2006-01-02 15:04:05-07")

	// compute the set of expired certificates
	expiredCertificateHashes, err := ExpiredCertificateHashes(timestamp, transaction, ctx)
	if err != nil {
		return nil, err
	}

	// remove all expired certificates
	_, err = transaction.Exec(ctx, "DELETE FROM certificates WHERE not_valid_after < '"+timestamp+"'")
	if err != nil {
		return nil, fmt.Errorf("failed deleting expired certificates: %v", err)
	}

	expiredCertificateHashesArray := bytesSliceToPostgresArray(expiredCertificateHashes)

	// '&&' checks for an intersection of arrays
	// see https://www.postgresql.org/docs/current/functions-array.html
	rows, err := transaction.Query(
		ctx,
		// requires an 'array_intersect' function to be defined
		"WITH sq AS (SELECT bit_string_51, bit_string_15, array_intersect(certificate_hashes, "+expiredCertificateHashesArray+") as expired_certificates "+
			"FROM nodes "+
			") "+
			"SELECT * FROM sq WHERE CARDINALITY(sq.expired_certificates) > 0",
	)
	if err != nil {
		return nil, fmt.Errorf("failed selecting nodes with expired certificates: %v", err)
	}

	defer rows.Close()

	// collect the certificate hashes to remove per bit string
	certificatesToRemove := make(map[bitstring.RawBitStringPair]([]crypto.SHA256Hash))

	for rows.Next() {
		var dbXYBitString pgtype.Bits
		var dbZBitString pgtype.Bits
		var dbExpiredCertificateHashes pgtype.Array[[]byte]

		err := rows.Scan(&dbXYBitString, &dbZBitString, &dbExpiredCertificateHashes)

		if err != nil {
			return nil, err
		}

		XYBitString := make([]byte, 8)
		copy(XYBitString, dbXYBitString.Bytes)

		ZBitString := make([]byte, 2)
		copy(ZBitString, dbZBitString.Bytes)

		certificatesToRemove[bitstring.RawBitStringPair{
			RawXYBitString: bitstring.RawXYBitString{
				XYBitString:    binary.BigEndian.Uint64(XYBitString),
				XYBitStringLen: uint8(dbXYBitString.Len),
			},
			RawZBitString: bitstring.RawZBitString{
				ZBitString:    binary.BigEndian.Uint16(ZBitString),
				ZBitStringLen: uint8(dbZBitString.Len),
			},
		}] = expiredCertificateHashes
	}

	err = rows.Err()
	if err != nil {
		return nil, err
	}

	return certificatesToRemove, nil
}

// returns a map from bit strings to the set of certificates that has to be added to the respective node
func AddCertificates(
	certificates []*crypto.GeoCertificate,
	fGrow float64,
	transaction pgx.Tx,
	ctx context.Context,
) (
	map[bitstring.RawBitStringPair]([]crypto.SHA256Hash),
	error,
) {
	certificatesToAdd := make(map[bitstring.RawBitStringPair]([]crypto.SHA256Hash))

	if len(certificates) <= 0 {
		return certificatesToAdd, nil
	}

	// start building insetion query
	query := "INSERT INTO certificates (certificate_hash, certificate, not_valid_after) VALUES "

	for i, certificate := range certificates {

		certificateHash := certificate.Hash()

		// append values to query for insertion

		// if not first, seperate by comma
		if i > 0 {
			query += ","
		}
		query += fmt.Sprintf(
			"(%s, %s, %s)",
			fmt.Sprintf("E'\\\\x%s'", hex.EncodeToString(certificateHash)),
			fmt.Sprintf("E'\\\\x%s'", hex.EncodeToString(certificate.Marshal())),
			fmt.Sprintf("'%s'", certificate.NotValidAfter),
		)

		// compute affected bit strings
		bitstrings, err := geometry.CertificateToBitStrings(certificate, fGrow)
		if err != nil {
			return nil, err
		}

		// and set respective bit string values in the map to mark them for insertion into the SMT
		for _, b := range bitstrings {
			// certificatesToAdd[*b] might be nil but go allows appending to nil -> new list
			// update list to contain (possibly) new pointer
			certificatesToAdd[*b] = append(certificatesToAdd[*b], certificateHash)
		}
	}

	// ignore duplicate certificates
	query += " ON CONFLICT (certificate_hash) DO NOTHING"

	_, err := transaction.Exec(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed inserting new certificates: %v", err)
	}

	return certificatesToAdd, nil
}

// adds new certificates and removes expired ones
// returns an SMH *WITHOUT* signature, call .Sign() on it
// and insert the result into the consistency tree
// pass time.Time{} do not remove any certificates
func UpdateTree(
	newCertificates []*crypto.GeoCertificate,
	fGrow float64,
	t time.Time,
	transaction pgx.Tx,
	coveredCTLogServers []crypto.CTLogServer,
	updateHashes bool,
	ctx context.Context,
) error {

	var certificatesToRemove map[bitstring.RawBitStringPair]([]crypto.SHA256Hash)
	var err error

	if t.IsZero() {
		// no certificates could have expired, skip removal
		certificatesToRemove = make(map[bitstring.RawBitStringPair]([]crypto.SHA256Hash))
	} else {
		// remove certificates & compute the set of bit strings / nodes that must change
		certificatesToRemove, err = RemoveExpiredCertificates(t, transaction, ctx)
		if err != nil {
			return err
		}
	}

	// add certificates & compute the set of bit strings / nodes that must change
	certificatesToAdd, err := AddCertificates(newCertificates, fGrow, transaction, ctx)
	if err != nil {
		return err
	}

	changeSet := mapset.NewSet[bitstring.RawBitStringPair]()
	for b := range certificatesToRemove {
		changeSet.Add(b)
	}
	for b := range certificatesToAdd {
		changeSet.Add(b)
	}

	for _, bitStringPair := range changeSet.ToSlice() {

		hashes := "nodes.certificate_hashes"

		removeSet, hasCertificatesToRemove := certificatesToRemove[bitStringPair]
		addSet, hasCertificatesToAdd := certificatesToAdd[bitStringPair]

		if hasCertificatesToRemove {
			// requires an 'array_difference' function to be defined
			hashes = fmt.Sprintf(
				// removes the 'removeSet' from 'hashes'
				"array_difference(%s,%s)",
				hashes,
				bytesSliceToPostgresArray(removeSet),
			)
		}

		if hasCertificatesToAdd {
			// requires an 'array_union' function to be defined
			hashes = fmt.Sprintf(
				// unions with the 'addSet' to 'hashes'
				"array_union(%s,%s)",
				hashes,
				bytesSliceToPostgresArray(addSet),
			)
		}

		// use null for the hashes if inserted new since it can only be a new sparse leaf
		// if it is new
		query := fmt.Sprintf(
			"INSERT INTO nodes(bit_string_51,bit_string_15,xy_left_child_hash,xy_right_child_hash,z_left_child_hash,z_right_child_hash,certificate_hashes) "+
				"VALUES(b'%s',b'%s',NULL,NULL,NULL,NULL,%s) "+
				"ON CONFLICT(bit_string_51,bit_string_15) DO UPDATE SET certificate_hashes=%s",
			// xy bit string
			bitStringPair.RawXYBitString.BitString().String(),
			// z bit string
			bitStringPair.RawZBitString.BitString().String(),
			// set of new certificates
			bytesSliceToPostgresArray(addSet),
			// for conflict update statement
			hashes,
		)

		// execute the row update
		_, err := transaction.Exec(ctx, query)
		if err != nil {
			return fmt.Errorf("failed updating certificate hashes: %v", err)
		}
	}

	// now all certificate updates have been performed, we need to recompute the hashes of all parents
	// compute the set of all parents
	ancestorSet := mapset.NewSet[bitstring.RawBitStringPair]()
	// the root node always has to be updated if there are changes
	ancestorSet.Add(bitstring.ROOT_NODE)

	for _, bitStringPair := range changeSet.ToSlice() {
		// compute all ancestors up to the root
		for ancestor := bitStringPair.ParentPair(); !ancestor.IsRoot(); ancestor = ancestor.ParentPair() {
			ancestorSet.Add(ancestor)
		}
	}

	ancestors := ancestorSet.ToSlice()

	sort.Slice(ancestors, func(i, j int) bool {
		// must return true if i is smaller than j (smaller = has longer bit strings)
		return ((ancestors[i].XYBitStringLen > ancestors[j].XYBitStringLen) || (ancestors[i].XYBitStringLen == ancestors[j].XYBitStringLen && ancestors[i].ZBitStringLen > ancestors[j].ZBitStringLen))
	})

	// sort the bit strings in ascending order (children to the root)
	for _, bitStringPair := range ancestors {

		// try to insert bit strings, do nothing if they are already there
		// can just use NULL for the hashes since they will be recomputed right after
		query := fmt.Sprintf(
			"INSERT INTO nodes(bit_string_51,bit_string_15,xy_left_child_hash,xy_right_child_hash,z_left_child_hash,z_right_child_hash,certificate_hashes) "+
				"VALUES (b'%s',b'%s',NULL,NULL,NULL,NULL,ARRAY[]::bytea[]) "+
				"ON CONFLICT DO NOTHING",
			bitStringPair.RawXYBitString.BitString().String(),
			bitStringPair.RawZBitString.BitString().String(),
		)

		_, err := transaction.Exec(ctx, query)
		if err != nil {
			return fmt.Errorf("failed inserting possibly non-existent ancestor: %v", err)
		}

		if updateHashes {

			// requires an 'update_children_hashes' function to be defined
			query = fmt.Sprintf(
				"SELECT update_children_hashes(b'%s', b'%s')",
				bitStringPair.RawXYBitString.BitString().String(),
				bitStringPair.RawZBitString.BitString().String(),
			)

			// execute the row update
			_, err = transaction.Exec(ctx, query)
			if err != nil {
				return fmt.Errorf("failed updating children hashes for ancestor: %v", err)
			}

		}
	}

	return nil
}
