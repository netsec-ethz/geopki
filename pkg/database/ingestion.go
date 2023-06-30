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
func findAndRemoveExpiredCertificates(
	certificatesToRemove map[bitstring.RawBitStringPair]([]crypto.SHA256Hash),
	t time.Time,
	transaction pgx.Tx,
	ctx context.Context,
) error {

	timestamp := t.Format("2006-01-02 15:04:05-07")

	// compute the set of expired certificates
	expiredCertificateHashes, err := ExpiredCertificateHashes(timestamp, transaction, ctx)
	if err != nil {
		return err
	}

	// remove all expired certificates
	_, err = transaction.Exec(ctx, "DELETE FROM certificates WHERE not_valid_after < '"+timestamp+"'")
	if err != nil {
		return fmt.Errorf("failed deleting expired certificates: %v", err)
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
		return fmt.Errorf("failed selecting nodes with expired certificates: %v", err)
	}

	defer rows.Close()

	// collect the certificate hashes to remove per bit string
	for rows.Next() {
		var dbXYBitString pgtype.Bits
		var dbZBitString pgtype.Bits
		var dbExpiredCertificateHashes pgtype.Array[[]byte]

		err := rows.Scan(&dbXYBitString, &dbZBitString, &dbExpiredCertificateHashes)

		if err != nil {
			return err
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
		return err
	}

	return nil
}

// returns a map from bit strings to the set of certificates that has to be added to the respective node
func insertCertificates(
	certificates []*crypto.GeoCertificate,
	certificatesToAdd map[bitstring.RawBitStringPair]([]crypto.SHA256Hash),
	fGrow float64,
	transaction pgx.Tx,
	ctx context.Context,
) error {

	if len(certificates) <= 0 {
		return nil
	}

	// start building insetion query
	var query strings.Builder
	query.WriteString("INSERT INTO certificates (certificate_hash, certificate, not_valid_after) VALUES ")

	for i, certificate := range certificates {

		certificateHash := certificate.Hash()

		// append values to query for insertion

		// if not first, seperate by comma
		if i > 0 {
			query.WriteString(",")
		}
		query.WriteString(
			fmt.Sprintf(
				"(%s, %s, %s)",
				fmt.Sprintf("E'\\\\x%s'", hex.EncodeToString(certificateHash)),
				fmt.Sprintf("E'\\\\x%s'", hex.EncodeToString(certificate.Marshal())),
				fmt.Sprintf("'%s'", certificate.NotValidAfter),
			),
		)

		// compute affected bit strings
		bitstrings, err := geometry.CertificateToBitStrings(certificate, fGrow)
		if err != nil {
			return err
		}

		// and set respective bit string values in the map to mark them for insertion into the SMT
		for _, b := range bitstrings {
			// certificatesToAdd[*b] might be nil but go allows appending to nil -> new list
			// update list to contain (possibly) new pointer
			certificatesToAdd[*b] = append(certificatesToAdd[*b], certificateHash)
		}
	}

	// ignore duplicate certificates
	query.WriteString(" ON CONFLICT (certificate_hash) DO NOTHING")

	_, err := transaction.Exec(ctx, query.String())
	if err != nil {
		return fmt.Errorf("failed inserting new certificates: %v", err)
	}

	return nil
}

// adds new certificates to the db
func AddNewCertificates(
	newCertificates []*crypto.GeoCertificate,
	fGrow float64,
	transaction pgx.Tx,
	coveredCTLogServers []crypto.CTLogServer,
	updateHashes bool,
	ctx context.Context,
) error {

	certificatesToAdd := make(map[bitstring.RawBitStringPair]([]crypto.SHA256Hash))

	// add certificates & compute the set of bit strings / nodes that must change
	err := insertCertificates(
		newCertificates,
		certificatesToAdd,
		fGrow,
		transaction,
		ctx,
	)
	if err != nil {
		return err
	}

	for bitStringPair := range certificatesToAdd {

		hashes := "nodes.certificate_hashes"
		addSet, hasCertificatesToAdd := certificatesToAdd[bitStringPair]

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
	ancestorSet := mapset.NewThreadUnsafeSet[bitstring.RawBitStringPair]()
	// the root node always has to be updated if there are changes
	ancestorSet.Add(bitstring.ROOT_NODE)

	for bitStringPair := range certificatesToAdd {
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

// removes expired certificates
func RemoveExpiredCertificates(
	t time.Time,
	transaction pgx.Tx,
	ctx context.Context,
) error {

	certificatesToRemove := make(map[bitstring.RawBitStringPair]([]crypto.SHA256Hash))
	var err error

	// remove certificates & compute the set of bit strings / nodes that must change
	err = findAndRemoveExpiredCertificates(certificatesToRemove, t, transaction, ctx)
	if err != nil {
		return err
	}

	for bitStringPair := range certificatesToRemove {

		hashes := "nodes.certificate_hashes"

		removeSet, hasCertificatesToRemove := certificatesToRemove[bitStringPair]

		if hasCertificatesToRemove {
			// requires an 'array_difference' function to be defined
			hashes = fmt.Sprintf(
				// removes the 'removeSet' from 'hashes'
				"array_difference(%s,%s)",
				hashes,
				bytesSliceToPostgresArray(removeSet),
			)
		}

		// use null for the hashes if inserted new since it can only be a new sparse leaf
		// if it is new
		query := fmt.Sprintf(
			"UPDATE nodes SET certificate_hashes=%s WHERE bit_string_51=%s AND bit_string_15=%s",
			// for conflict update statement
			hashes,
			// xy bit string
			bitStringPair.RawXYBitString.BitString().String(),
			// z bit string
			bitStringPair.RawZBitString.BitString().String(),
		)

		// execute the row update
		_, err := transaction.Exec(ctx, query)
		if err != nil {
			return fmt.Errorf("failed updating certificate hashes: %v", err)
		}
	}

	// now all certificate updates have been performed, we need to recompute the hashes of all parents
	// compute the set of all parents
	ancestorSet := mapset.NewThreadUnsafeSet[bitstring.RawBitStringPair]()
	// the root node always has to be updated if there are changes
	ancestorSet.Add(bitstring.ROOT_NODE)

	for bitStringPair := range certificatesToRemove {
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

		// node must already be in the db, all ancestors are created on insertion

		// requires an 'update_children_hashes' function to be defined
		query := fmt.Sprintf(
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

	return nil
}
