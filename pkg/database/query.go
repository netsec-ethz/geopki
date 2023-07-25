package database

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"math"
	"strconv"
	"strings"
	"sync"

	"geopki/pkg/bitstring"
	"geopki/pkg/comm"
	"geopki/pkg/crypto"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var stringBuilderPool = sync.Pool{
	New: func() any {
		// The Pool's New function should generally only return pointer
		// types, since a pointer can be put into the return interface
		// value without an allocation:
		return new(strings.Builder)
	},
}

var pointQueryPool = sync.Pool{
	New: func() any {
		// The Pool's New function should generally only return pointer
		// types, since a pointer can be put into the return interface
		// value without an allocation:
		return mapset.NewThreadUnsafeSet[string]()
	},
}

func BuildNodeQuery(bitStrings []*comm.XYBitString, minAltitude, maxAltitude uint16) string {
	// generate a query for each requested bit string pair
	// and put them in an slice
	query := stringBuilderPool.Get().(*strings.Builder)

	// collect point queries over all bit string pairs
	point_queries := pointQueryPool.Get().(mapset.Set[string])

	defer func() {
		query.Reset()
		stringBuilderPool.Put(query)

		point_queries.Clear()
		pointQueryPool.Put(point_queries)
	}()

	query.WriteString("(")

	for i, bitString := range bitStrings {
		// compute all prefixes of bitString that are not obtained by removing a trailing zero

		// 1. convert to string
		s := strconv.FormatUint(bitString.XYBitString, 2)
		// 2. left pad to full width of 64 bits, 3. remove trailing zeros,
		trimmedXYBitString := strings.TrimRight(
			strings.Repeat("0", 64-len(s))+s,
			"0",
		)

		// add all proper prefixes of 'trimmedXYBitString' except the empty string
		pointQueriesCount := len(trimmedXYBitString)
		for i := 1; i < pointQueriesCount; i++ {
			point_queries.Add(trimmedXYBitString[:i])
		}

		// clear all unused bits, i.e. extend the bit string to 64 bits with zeros
		// then shift it to the right to only take into account the 51 bits we're interested in
		bitStringMinInt := bitString.XYBitString & (uint64(math.MaxUint64) << (64 - bitString.XYBitStringLen))
		// to interpret is as a (big-endian) integer, we shift it to the right by 64 - 51 bits
		// previously the relevant 51 bits were at the beginning of the 64 bits, afterwards the
		// are at the end
		bitStringMinInt = bitStringMinInt >> (64 - 51)

		// same as before but now we set all unused bits, i.e. extend the bit string to 64 bits with ones
		bitStringMaxInt := bitString.XYBitString | (uint64(math.MaxUint64) >> bitString.XYBitStringLen)
		bitStringMaxInt = bitStringMaxInt >> (64 - 51)

		// in general fmt.Sprintf is not prone to SQL injections but since the user input is
		// checked against the protobuf format and interpreted as integers
		// using prepared statements would be an option too but it would have to be generated on
		// the fly. thus an easier improvement to ensure this stays safe would be to use a
		// postgres function
		if i > 0 {
			query.WriteString("UNION")
		}
		// integer range query
		query.WriteString("(SELECT bit_string_51, bit_string_15, xy_left_child_hash, xy_right_child_hash, z_left_child_hash, z_right_child_hash, certificate_hashes FROM nodes WHERE bit_string_51_int >= ")
		query.WriteString(strconv.FormatUint(bitStringMinInt, 10))
		query.WriteString(" AND bit_string_51_int <= ")
		query.WriteString(strconv.FormatUint(bitStringMaxInt, 10))
		query.WriteString(" AND altitude_min <= ")
		// note that we subtract one because 2^15 cannot be represented using signed smallint
		// therefore we shifted the whole range down by 1 from [0, 32'768] to [-1, 32'767]
		query.WriteString(strconv.FormatInt(int64(maxAltitude)-1, 10))
		query.WriteString(" AND altitude_max >= ")
		query.WriteString(strconv.FormatInt(int64(minAltitude)-1, 10))
		query.WriteString(")")
	}

	// union, not union all. while for individual queries there won't be any overlap
	// across multiple bit strings there can be.
	// example: query for [100100, 10011]
	// then '1001' is a prefix of both and will be included twice. onece because of the range query for 100100 and once because it is a prefix of 10011
	// also always query the root node
	query.WriteString(") UNION SELECT bit_string_51, bit_string_15, xy_left_child_hash, xy_right_child_hash, z_left_child_hash, z_right_child_hash, certificate_hashes FROM nodes WHERE bit_string_51 IN (''")
	// add all other prefixes of 'trimmedXYBitString'
	for bitString := range point_queries.Iter() {
		query.WriteString(",'" + bitString + "'")
	}
	// note that we subtract one because 2^15 cannot be represented using signed smallint
	// therefore we shifted the whole range down by 1 from [0, 32'768] to [-1, 32'767]
	query.WriteString(") AND altitude_min <= ")
	query.WriteString(strconv.FormatInt(int64(maxAltitude)-1, 10))
	query.WriteString(" AND altitude_max >= ")
	query.WriteString(strconv.FormatInt(int64(minAltitude)-1, 10))

	return query.String()
}

func QueryRootHash(
	transaction pgx.Tx,
	ctx context.Context,
) (crypto.SHA256Hash, error) {

	row := transaction.QueryRow(
		ctx,
		"SELECT xy_left_child_hash, xy_right_child_hash, z_left_child_hash, z_right_child_hash, certificate_hashes "+
			"FROM nodes "+
			"WHERE bit_string_51=b'' AND bit_string_15=b''",
	)

	var dbXYLeftChildHash, dbXYRightChildHash, dbZLeftChildHash, dbZRightChildHash []byte
	var dbCertificateHashes pgtype.Array[[]byte]

	err := row.Scan(
		&dbXYLeftChildHash,
		&dbXYRightChildHash,
		&dbZLeftChildHash,
		&dbZRightChildHash,
		&dbCertificateHashes,
	)

	if err != nil {
		return nil, err
	}

	// create new node instance from loaded data
	node := crypto.NewNode(
		// root node has zero length for both
		0, 0, 0, 0,

		dbXYLeftChildHash,
		dbXYRightChildHash,
		dbZLeftChildHash,
		dbZRightChildHash,

		dbCertificateHashes.Elements,
	)

	return node.Hash(), nil
}

var bitStringSetPool = sync.Pool{
	New: func() any {
		// The Pool's New function should generally only return pointer
		// types, since a pointer can be put into the return interface
		// value without an allocation:
		return mapset.NewThreadUnsafeSet[bitstring.RawBitStringPair]()
	},
}

func RowsToNodesAndRootHash(
	rows pgx.Rows,
	expectedResults int,
) ([]*comm.Node, crypto.SHA256Hash, error) {
	nodes := make([]*crypto.Node, 0, expectedResults)

	// create a set of bit string pairs
	bitStringSet := bitStringSetPool.Get().(mapset.Set[bitstring.RawBitStringPair])
	defer func() {
		bitStringSet.Clear()
		bitStringSetPool.Put(bitStringSet)
	}()

	var rootHash crypto.SHA256Hash

	var dbXYBitString pgtype.Bits
	var dbZBitString pgtype.Bits
	// use a pointer to a byte array to allow for null values
	var dbXYLeftChildHash, dbXYRightChildHash, dbZLeftChildHash, dbZRightChildHash []byte
	var dbCertificateHashes pgtype.Array[[]byte]

	XYBitString := make([]byte, 8)
	ZBitString := make([]byte, 2)

	// Iterate through the result set
	for rows.Next() {

		err := rows.Scan(
			&dbXYBitString,
			&dbZBitString,
			&dbXYLeftChildHash,
			&dbXYRightChildHash,
			&dbZLeftChildHash,
			&dbZRightChildHash,
			&dbCertificateHashes,
		)

		if err != nil {
			return nil, nil, err
		}

		// grow bit strings to 8 and 2 byte arrays respectively
		// by copy the contents and clearing the remaining bytes
		copy(XYBitString, dbXYBitString.Bytes)
		for i := len(dbXYBitString.Bytes); i < len(XYBitString); i++ {
			XYBitString[i] = 0
		}

		copy(ZBitString, dbZBitString.Bytes)
		for i := len(dbZBitString.Bytes); i < len(ZBitString); i++ {
			ZBitString[i] = 0
		}

		// create new node instance from loaded data
		node := crypto.NewNode(
			binary.BigEndian.Uint64(XYBitString),
			uint8(dbXYBitString.Len),

			binary.BigEndian.Uint16(ZBitString),
			uint8(dbZBitString.Len),

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
	}

	// Any errors encountered by rows.Next or rows.Scan will be returned here
	err := rows.Err()
	if err != nil {
		return nil, nil, err
	}

	responseNodes := make([]*comm.Node, len(nodes))

	for i, node := range nodes {

		// check whether each of the children are in 'bitStringSet', i.e.
		// will thus be returned to the user. if they are, the respective hash
		// does not have to be included in the response and can be set to nil
		// this loop could be omitted increasing the performance but increasing the response size

		child, err := node.XYLeftChildPair()
		// if err != nil, the child does not exist and the bit string can be cleared
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
			XYLeftChildHash:   node.XYLeftChildHash(false),
			XYRightChildHash:  node.XYRightChildHash(false),
			ZLeftChildHash:    node.ZLeftChildHash(false),
			ZRightChildHash:   node.ZRightChildHash(false),
			CertificateHashes: node.CertificateHashes,
		}

		responseNodes[i] = responseNode
	}

	return responseNodes, rootHash, nil
}

// accepts a set of base64 encoded certificate hashes
func BuildCertificateQuery(certificateStringHashes mapset.Set[string]) (string, error) {
	var query strings.Builder
	query.WriteString("SELECT certificate FROM certificates WHERE certificate_hash IN(")

	i := 0
	for certificateStringHash := range certificateStringHashes.Iter() {
		certificateHash, err := base64.RawURLEncoding.DecodeString(certificateStringHash)
		if err != nil {
			return "", err
		}

		if i > 0 {
			query.WriteString(",")
		}

		query.WriteString("E'\\\\x")
		query.WriteString(hex.EncodeToString(certificateHash))
		query.WriteString("'")
		i++
	}
	query.WriteString(")")

	return query.String(), nil
}

func RowsToCertificates(
	rows pgx.Rows,
	expectedResults int,
) ([][]byte, error) {
	certificates := make([][]byte, 0, expectedResults)

	var dbCertificate []byte

	// Iterate through the result set
	for rows.Next() {
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
