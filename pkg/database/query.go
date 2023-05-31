package database

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"

	"geopki/pkg/bitstring"
	"geopki/pkg/comm"
	"geopki/pkg/crypto"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func BuildNodeQueries(bitStrings []*comm.XYBitString, minAltitude, maxAltitude uint16) []string {
	// generate a query for each requested bit string pair
	// and put them in an slice
	queries := make([]string, len(bitStrings))

	for i, bitStringPair := range bitStrings {
		// compute all prefixes of bitString that are not obtained by removing a trailing zero
		// 1. convert to string 2. remove trailing zeros, 3. compute all prefixes
		trimmedXYBitString := strings.TrimRight(
			strconv.FormatUint(bitStringPair.XYBitString, 2),
			"0",
		)

		pointQueriesCount := len(trimmedXYBitString) + 1
		pointQueries := make([]string, pointQueriesCount)
		for i := 0; i < pointQueriesCount; i++ {
			pointQueries[i] = fmt.Sprintf("b'%s'", trimmedXYBitString[:i])
		}

		// clear all unused bits, i.e. extend the bit string to 64 bits with zeros
		// then shift it to the right to only take into account the 51 bits we're interested in
		bitStringMinInt := bitStringPair.XYBitString & (uint64(math.MaxUint64) << (64 - bitStringPair.XYBitStringLen))
		// to interpret is as a (big-endian) integer, we shift it to the right by 64 - 51 bits
		// previously the relevant 51 bits were at the beginning of the 64 bits, afterwards the
		// are at the end
		bitStringMinInt = bitStringMinInt >> (64 - 51)

		// same as before but now we set all unused bits, i.e. extend the bit string to 64 bits with ones
		bitStringMaxInt := bitStringPair.XYBitString | (uint64(math.MaxUint64) >> (64 - bitStringPair.XYBitStringLen))
		bitStringMaxInt = bitStringMaxInt >> (64 - 51)

		// in general fmt.Sprintf is not prone to SQL injections but since the user input is
		// checked against the protobuf format and interpreted as integers
		// using prepared statements would be an option too but it would have to be generated on
		// the fly. thus an easier improvement to ensure this stays safe would be to use a
		// postgres function
		queries[i] = fmt.Sprintf(
			"(SELECT bit_string_51, bit_string_15, neighbor_hash, xy_left_child_hash, xy_right_child_hash, z_left_child_hash, z_right_child_hash, certificate_hashes "+
				"FROM nodes "+
				"WHERE bit_string_51 IN (%s) AND "+
				"altitude_min <= %d AND "+
				"altitude_max >= %d"+
				"UNION ALL"+
				" "+
				"SELECT bit_string_51, bit_string_15, neighbor_hash, xy_left_child_hash, xy_right_child_hash, z_left_child_hash, z_right_child_hash, certificate_hashes "+
				"FROM nodes "+
				"WHERE "+
				"bit_string_51_int >= %d AND "+
				"bit_string_51_int <= %d AND "+
				"altitude_min <= %[2]d AND "+
				"altitude_max >= %[3]d"+
				")",
			strings.Join(pointQueries, ","),
			minAltitude,
			maxAltitude,
			bitStringMinInt,
			bitStringMaxInt,
		)
	}

	return queries
}

func BuildNodeQuery(bitStrings []*comm.XYBitString, minAltitude, maxAltitude uint16) string {
	return strings.Join(BuildNodeQueries(bitStrings, minAltitude, maxAltitude), "UNION")
}

func RowsToNodesAndRootHash(
	rows pgx.Rows,
	expectedResults int,
) ([]*comm.Node, crypto.SHA256Hash, mapset.Set[string], error) {
	nodes := make([]*crypto.Node, 0, expectedResults)

	// create a set of bit string pairs
	bitStringSet := mapset.NewSet[bitstring.RawBitStringPair]()

	var rootHash crypto.SHA256Hash
	certificateStringHashes := mapset.NewSet[string]()

	// Iterate through the result set
	for rows.Next() {
		var dbXYBitString pgtype.Bits
		var dbZBitString pgtype.Bits
		// use a pointer to a byte array to allow for null values
		var dbNeighborHash, dbXYLeftChildHash, dbXYRightChildHash, dbZLeftChildHash, dbZRightChildHash []byte
		var dbCertificateHashes pgtype.Array[[]byte]

		err := rows.Scan(
			&dbXYBitString,
			&dbZBitString,
			&dbNeighborHash,
			&dbXYLeftChildHash,
			&dbXYRightChildHash,
			&dbZLeftChildHash,
			&dbZRightChildHash,
			&dbCertificateHashes,
		)

		if err != nil {
			return nil, nil, nil, err
		}

		// grow bit strings to 8 and 2 byte arrays respectively
		// by allocating a 8 and a 2 byte array and copy the contents
		XYBitString := make([]byte, 8)
		copy(XYBitString, dbXYBitString.Bytes)

		ZBitString := make([]byte, 2)
		copy(ZBitString, dbZBitString.Bytes)

		// create new node instance from loaded data
		node := crypto.NewDBNode(
			binary.BigEndian.Uint64(XYBitString),
			uint8(dbXYBitString.Len),

			binary.BigEndian.Uint16(ZBitString),
			uint8(dbZBitString.Len),

			dbNeighborHash,
			dbXYLeftChildHash,
			dbXYRightChildHash,
			dbZLeftChildHash,
			dbZRightChildHash,

			dbCertificateHashes.Elements,
		)
		// append new instance to the list, will be returned to the client after
		// some additional processing
		nodes = append(nodes, node)

		// add bit string pair to the set
		bitStringSet.Add(node.Pair())

		// if it is the root, remember it
		if node.IsRoot() {
			rootHash = node.Hash()
		}

		// collect certificate hashes
		for _, certificateHash := range dbCertificateHashes.Elements {
			certificateStringHashes.Add(hex.EncodeToString(certificateHash))
		}
	}

	// Any errors encountered by rows.Next or rows.Scan will be returned here
	err := rows.Err()
	if err != nil {
		return nil, nil, nil, err
	}

	responseNodes := make([]*comm.Node, len(nodes))

	for i, node := range nodes {

		// check if the neighbor and each of the children are in 'bitStringSet', i.e.
		// will thus be returned to the user. if they are, the respective hash
		// does not have to be included in the response and can be set to nil
		// this loop could be omitted increasing the performance but increasing the response size

		if bitStringSet.Contains(node.NeighborPair()) {
			node.ClearNeighbor()
		}

		child, err := node.XYLeftChildPair()
		if err != nil || bitStringSet.Contains(child) {
			node.ClearXYLeftChild()
		}

		child, err = node.XYRightChildPair()
		if err != nil || bitStringSet.Contains(child) {
			node.ClearXYRightChild()
		}

		if bitStringSet.Contains(node.ZLeftChildPair()) {
			node.ClearZLeftChild()
		}
		if bitStringSet.Contains(node.ZRightChildPair()) {
			node.ClearZRightChild()
		}

		responseNode := &comm.Node{
			XYBitString:    node.XYBitString,
			XYBitStringLen: uint32(node.XYBitStringLen),
			ZBitString:     uint32(node.ZBitString),
			ZBitStringLen:  uint32(node.ZBitStringLen),
			// do not fill with default hashes, can be omitted for smaller response sizes
			NeighborHash:      node.NeighborHash(false),
			XYLeftChildHash:   node.XYLeftChildHash(false),
			XYRightChildHash:  node.XYRightChildHash(false),
			ZLeftChildHash:    node.ZLeftChildHash(false),
			ZRightChildHash:   node.ZRightChildHash(false),
			CertificateHashes: node.CertificateHashes,
		}

		responseNodes[i] = responseNode
	}

	return responseNodes, rootHash, certificateStringHashes, nil
}

func BuildCertificateQuery(certificateStringHashes []string) string {
	encodedHashes := make([]string, len(certificateStringHashes))

	for i, certificateStringHash := range certificateStringHashes {
		encodedHashes[i] = fmt.Sprintf("E'\\\\x%s'", certificateStringHash)
	}

	return fmt.Sprintf(
		"SELECT certificate "+
			"FROM certificates "+
			"WHERE certificate_hash IN (%s)",
		strings.Join(encodedHashes, ","),
	)
}

func RowsToCertificates(
	rows pgx.Rows,
	expectedResults int,
) ([][]byte, error) {
	certificates := make([][]byte, 0, expectedResults)

	// Iterate through the result set
	for rows.Next() {
		var dbCertificate []byte

		err := rows.Scan(
			&dbCertificate,
		)

		if err != nil {
			return nil, err
		}

		certificates = append(certificates, dbCertificate)
	}

	// Any errors encountered by rows.Next or rows.Scan will be returned here
	err := rows.Err()
	if err != nil {
		return nil, err
	}

	return certificates, nil
}
