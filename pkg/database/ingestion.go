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
	"sync"
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
func expiredCertificateHashes(
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
	expiredCertificateHashes, err := expiredCertificateHashes(timestamp, transaction, ctx)
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
			"FROM nodes_next "+
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

var addCertificatesQueryStringBuilderPool = sync.Pool{
	New: func() any {
		// The Pool's New function should generally only return pointer
		// types, since a pointer can be put into the return interface
		// value without an allocation:
		return new(strings.Builder)
	},
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
	query := addCertificatesQueryStringBuilderPool.Get().(*strings.Builder)
	defer func() {
		query.Reset()
		addCertificatesQueryStringBuilderPool.Put(query)
	}()

	// new certificates can be directly inserted into the certificates table
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

var certificatesToAddPool = sync.Pool{
	New: func() any {
		// The Pool's New function should generally only return pointer
		// types, since a pointer can be put into the return interface
		// value without an allocation:
		return make(map[bitstring.RawBitStringPair]([]crypto.SHA256Hash))
	},
}

var certificatesToRemovePool = sync.Pool{
	New: func() any {
		// The Pool's New function should generally only return pointer
		// types, since a pointer can be put into the return interface
		// value without an allocation:
		return make(map[bitstring.RawBitStringPair]([]crypto.SHA256Hash))
	},
}

var changeSetPool = sync.Pool{
	New: func() any {
		// The Pool's New function should generally only return pointer
		// types, since a pointer can be put into the return interface
		// value without an allocation:
		return mapset.NewThreadUnsafeSet[bitstring.RawBitStringPair]()
	},
}

var ancestorSetPool = sync.Pool{
	New: func() any {
		// The Pool's New function should generally only return pointer
		// types, since a pointer can be put into the return interface
		// value without an allocation:
		return mapset.NewThreadUnsafeSet[bitstring.RawBitStringPair]()
	},
}

// adds new certificates to the db
// does *not* modify the 'nodes' table, inserts into 'nodes_next' to batch the updates
func AddNewCertificates(
	newCertificates []*crypto.GeoCertificate,
	fGrow float64,
	transaction pgx.Tx,
	ctx context.Context,
) error {

	certificatesToAdd := certificatesToAddPool.Get().(map[bitstring.RawBitStringPair]([]crypto.SHA256Hash))
	var err error

	defer func() {
		// empty map
		for key := range certificatesToAdd {
			delete(certificatesToAdd, key)
		}

		// give it back to the memory pool
		certificatesToAddPool.Put(certificatesToAdd)
	}()

	// add certificates & compute the set of bit strings / nodes that must change
	err = insertCertificates(
		newCertificates,
		certificatesToAdd,
		fGrow,
		transaction,
		ctx,
	)
	if err != nil {
		return err
	}

	// now all certificate updates have been performed, we need to recompute the hashes of all parents
	// compute the set of all parents
	ancestorSet := changeSetPool.Get().(mapset.Set[bitstring.RawBitStringPair])
	defer func() {
		ancestorSet.Clear()
		ancestorSetPool.Put(ancestorSet)
	}()
	// the root node always has to be updated if there are changes
	ancestorSet.Add(bitstring.ROOT_NODE)

	for bitStringPair := range certificatesToAdd {

		addSet := certificatesToAdd[bitStringPair]
		// requires an 'array_union' function to be defined
		hashes := fmt.Sprintf(
			// unions with the 'addSet' to 'hashes'
			"array_union(nodes_next.certificate_hashes,%s)",
			bytesSliceToPostgresArray(addSet),
		)

		// use null for the hashes if inserted new since it can only be a new sparse leaf
		// if it is new
		query := fmt.Sprintf(
			"INSERT INTO nodes_next(bit_string_51,bit_string_15,xy_left_child_hash,xy_right_child_hash,z_left_child_hash,z_right_child_hash,certificate_hashes) "+
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

		// compute all ancestors up to the root
		for ancestor := bitStringPair.ParentPair(); !ancestor.IsRoot(); ancestor = ancestor.ParentPair() {
			ancestorSet.Add(ancestor)
		}
	}

	// convert ancestors to a slice for proper update order (upwards from the bottom)
	ancestors := ancestorSet.ToSlice()
	sort.Slice(ancestors, func(i, j int) bool {
		// must return true if i is smaller than j (smaller = has longer bit strings)
		return ((ancestors[i].XYBitStringLen > ancestors[j].XYBitStringLen) || (ancestors[i].XYBitStringLen == ancestors[j].XYBitStringLen && ancestors[i].ZBitStringLen > ancestors[j].ZBitStringLen))
	})

	for _, bitStringPair := range ancestors {

		// try to insert bit strings, do nothing if they are already there
		// can just use NULL for the hashes since they will be recomputed right after
		query := fmt.Sprintf(
			"INSERT INTO nodes_next(bit_string_51,bit_string_15,xy_left_child_hash,xy_right_child_hash,z_left_child_hash,z_right_child_hash,certificate_hashes) "+
				"VALUES (b'%s',b'%s',NULL,NULL,NULL,NULL,ARRAY[]::bytea[]) "+
				"ON CONFLICT DO NOTHING",
			bitStringPair.RawXYBitString.BitString().String(),
			bitStringPair.RawZBitString.BitString().String(),
		)

		_, err := transaction.Exec(ctx, query)
		if err != nil {
			return fmt.Errorf("failed inserting possibly non-existent ancestor: %v", err)
		}

		// requires an 'update_children_hashes' and a 'smt_hash' function to be defined
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

	return nil
}

// removes expired certificates from 'nodes_next'
func RemoveExpiredCertificates(
	t time.Time,
	transaction pgx.Tx,
	ctx context.Context,
) error {

	certificatesToRemove := certificatesToRemovePool.Get().(map[bitstring.RawBitStringPair]([]crypto.SHA256Hash))
	var err error

	defer func() {
		// empty map
		for key := range certificatesToRemove {
			delete(certificatesToRemove, key)
		}

		// give it back to the memory pool
		certificatesToRemovePool.Put(certificatesToRemove)
	}()

	// remove certificates & compute the set of bit strings / nodes that must change
	err = findAndRemoveExpiredCertificates(certificatesToRemove, t, transaction, ctx)
	if err != nil {
		return err
	}

	// compute the set of all ancestors
	ancestorSet := changeSetPool.Get().(mapset.Set[bitstring.RawBitStringPair])
	defer func() {
		ancestorSet.Clear()
		ancestorSetPool.Put(ancestorSet)
	}()
	// the root node always has to be updated if there are changes
	ancestorSet.Add(bitstring.ROOT_NODE)

	// iterate bit string pairs, update hashes and compute set of ancestors

	for bitStringPair := range certificatesToRemove {

		hashes := "nodes_next.certificate_hashes"

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

		query := fmt.Sprintf(
			"UPDATE nodes_next SET certificate_hashes=%s WHERE bit_string_51=%s AND bit_string_15=%s",
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

		// iterate over ancestors and add them to the set
		for ancestor := bitStringPair.ParentPair(); !ancestor.IsRoot(); ancestor = ancestor.ParentPair() {
			ancestorSet.Add(ancestor)
		}
	}

	// now all certificate updates have been performed, we need to recompute the hashes of all ancestors

	// convert ancestors to a slice for proper update order (upwards from the bottom)
	ancestors := ancestorSet.ToSlice()
	sort.Slice(ancestors, func(i, j int) bool {
		// must return true if i is smaller than j (smaller = has longer bit strings)
		return ((ancestors[i].XYBitStringLen > ancestors[j].XYBitStringLen) || (ancestors[i].XYBitStringLen == ancestors[j].XYBitStringLen && ancestors[i].ZBitStringLen > ancestors[j].ZBitStringLen))
	})

	for _, bitStringPair := range ancestors {
		// nodes are already in the db, all ancestors are created on insertion

		// requires an 'update_children_hashes' and a 'smt_hash' function to be defined
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
