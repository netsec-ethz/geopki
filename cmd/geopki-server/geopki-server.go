package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"

	"geopki/pkg/bitstring"
	"geopki/pkg/comm"
	"geopki/pkg/database"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"
)

// 'constants', arrays can't be set to 'const' though!
var TRUSTED_PROXIES = []string{"localhost"}
var DEFAULT_HASH = []byte("\x6e\x34\x0b\x9c\xff\xb3\x7a\x98\x9c\xa5\x44\xe6\xbb\x78\x0a\x2c\x78\x90\x1d\x3f\xb3\x37\x38\x76\x85\x11\xa3\x06\x17\xaf\xa0\x1d")

const (
	PREPARED_STATEMENT_QUERY_BITSTRINGS = "query_bit_strings"
)

type EndpointHandlerEnv struct {
	dbPool *pgxpool.Pool
}

func main() {

	listenAddress := "0.0.0.0"
	listenPort := 1234

	if len(os.Getenv("DATABASE_URL")) == 0 {
		fmt.Fprintf(os.Stderr, "No 'DATABASE_URL' environment variable provided\n")
		os.Exit(1)
	}

	// create a connection pool
	dbPool, err := pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to create connection pool: %v\n", err)
		os.Exit(2)
	}

	// check if the connection is working
	err = dbPool.Ping(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connect to the database: %v\n", err)
		os.Exit(3)
	}

	defer dbPool.Close()

	r := gin.Default()

	// configure gin engine
	r.SetTrustedProxies(TRUSTED_PROXIES)

	// create handler environment
	env := &EndpointHandlerEnv{
		dbPool: dbPool,
	}

	// install endpoints
	r.POST("/v1/get-bit-strings", env.getBitStrings)

	// start server
	r.Run(fmt.Sprintf("%s:%d", listenAddress, listenPort))
}

// handler for the /get-bit-strings endpoint
func (env *EndpointHandlerEnv) getBitStrings(c *gin.Context) {
	// check the content type request header
	contentTypeHeaders, ok := c.Request.Header["Content-Type"]
	if ok {
		// if the content type header is set, make sure it is exactly 'application/octet-stream'
		if len(contentTypeHeaders) > 1 || contentTypeHeaders[0] != "application/octet-stream" {

			// display an error to the user
			c.JSON(http.StatusOK, gin.H{
				"error": fmt.Sprintf(
					"Content-Type:%s is not supported. Don't set the header or use 'application/octet-stream'.",
					contentTypeHeaders[0],
				),
			})

			return
		}
	}

	// read request body
	// request, err := io.ReadAll(c.Request.Body)
	plainRequest, err := io.ReadAll(c.Request.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Reading request body failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	request := new(comm.Request)
	err = proto.Unmarshal(plainRequest, request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	if request.MaxAltitude > uint32(bitstring.C_Z) || request.MinAltitude > uint32(bitstring.C_Z) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("altitude values cannot be greater than %d", bitstring.C_Z),
		})
		return
	}

	requestBitStringPairs := request.GetXYBitStringPairs()

	if len(requestBitStringPairs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "requested no xy bit string pairs",
		})
		return
	}

	for i, requestBitStringPairs := range requestBitStringPairs {
		if requestBitStringPairs.XYBitStringLen > 51 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("bit string length of at index %d is %d which is > 51", i, requestBitStringPairs.XYBitStringLen),
			})
			return
		}
	}

	var minAltitude uint16 = uint16(request.MinAltitude)
	var maxAltitude uint16 = uint16(request.MaxAltitude)
	// conn, err := env.dbPool.Acquire(context.Background())
	// if err != nil {
	// 	fmt.Fprintf(os.Stderr, "Acquiring a connection from the pool failed: %v\n", err)
	// 	c.JSON(http.StatusInternalServerError, gin.H{
	// 		"error": "database connection could not be obtained, check the server logs",
	// 	})
	// 	return
	// }

	// conn.Conn().Prepare(
	// 	context.Background(),
	// 	PREPARED_STATEMENT_QUERY_BITSTRINGS,
	// 	"SELECT bit_string_51, bit_string_15, certificate_hashes, neighbor_hash, xy_left_child_hash, xy_right_child_hash, z_left_child_hash, z_right_child_hash "+
	// 		"FROM nodes "+
	// 		"WHERE bit_string_51 = ANY($1) AND "+
	// 		"altitude_min <= $2 AND "+
	// 		"altitude_max >= $3"+
	// 		"UNION ALL "+
	// 		"SELECT bit_string_51, bit_string_15, certificate_hashes, neighbor_hash, xy_left_child_hash, xy_right_child_hash, z_left_child_hash, z_right_child_hash "+
	// 		"FROM nodes "+
	// 		"WHERE "+
	// 		"bit_string_51_int >= $4 AND "+
	// 		"bit_string_51_int <= $5 AND "+
	// 		"altitude_min <= $2 AND "+
	// 		"altitude_max >= $3",
	// )

	// generate a query for each requested bit string pair
	// and put them in an slice
	queries := make([]string, len(requestBitStringPairs))
	for i, bitStringPair := range requestBitStringPairs {
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
		bitStringMinInt := bitStringPair.XYBitString & (math.MaxUint64 << (64 - bitStringPair.XYBitStringLen))
		// to interpret is as a (big-endian) integer, we shift it to the right by 64 - 51 bits
		// previously the relevant 51 bits were at the beginning of the 64 bits, afterwards the
		// are at the end
		bitStringMinInt = bitStringMinInt >> (64 - 51)

		// same as before but now we set all unused bits, i.e. extend the bit string to 64 bits with ones
		bitStringMaxInt := bitStringPair.XYBitString | (math.MaxUint64 >> (64 - bitStringPair.XYBitStringLen))
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

	rows, err := env.dbPool.Query(
		context.Background(),
		strings.Join(queries, "UNION"),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Query failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "database query failed, check the server logs",
		})
		return
	}

	defer rows.Close()

	// at least allocate a capacity of 'len(bit_strings)', then let
	// the go standard libary handle growth
	nodes := make([]database.Node, 0, len(requestBitStringPairs))

	// create a set of bit string pairs
	bitStringSet := mapset.NewSet[bitstring.RawBitStringPair]()

	// these two byte arrays are re-used multiple times in the following
	// allows for instance conversion between integers and byte arrays as well
	XYBitString := make([]byte, 8)
	ZBitString := make([]byte, 2)

	// Iterate through the result set
	for rows.Next() {
		var dbXYBitString pgtype.Bits
		var dbZBitString pgtype.Bits
		// use a pointer to a byte array to allow for null values
		var dbNeighborHash, dbXYLeftChildHash, dbXYRightChildHash, dbZLeftChildHash, dbZRightChildHash []byte
		var dbCertificateHashes pgtype.Array[[]byte]

		err = rows.Scan(
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
			fmt.Fprintf(os.Stderr, "Scanning row failed: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "scanning row failed, check the server logs",
			})
			return
		}

		// grow bit strings to 8 and 2 byte arrays respectively
		// by allocating a 8 and a 2 byte array and copy the contents
		copy(XYBitString, dbXYBitString.Bytes)
		copy(ZBitString, dbZBitString.Bytes)

		// create new node instance from loaded data
		node := database.Node{
			RawBitStringPair: bitstring.RawBitStringPair{
				XYBitString:    binary.BigEndian.Uint64(XYBitString),
				XYBitStringLen: uint8(dbXYBitString.Len),
				ZBitString:     binary.BigEndian.Uint16(ZBitString),
				ZBitStringLen:  uint8(dbZBitString.Len),
			},

			XYLeftChildHash:  dbXYLeftChildHash,
			XYRightChildHash: dbXYRightChildHash,
			ZLeftChildHash:   dbZLeftChildHash,
			ZRightChildHash:  dbZRightChildHash,

			NeighborHash:      dbNeighborHash,
			CertificateHashes: dbCertificateHashes.Elements,
		}
		// append new instance to the list, will be returned to the client after
		// some additional processing
		nodes = append(nodes, node)

		// add bit string pair to the set
		bitStringSet.Add(node.Pair())
	}

	var responseNodes []*comm.Node

	for _, node := range nodes {

		// check if the neighbor and each of the children are in 'bitStringSet', i.e.
		// will thus be returned to the user. if they are, the respective hash
		// does not have to be included in the response and can be set to nil
		// this loop could be omitted increasing the performance but increasing the response size

		if node.NeighborHash != nil && bitStringSet.Contains(node.NeighborPair()) {
			node.NeighborHash = nil
		}

		if (node.XYLeftChildHash != nil && bitStringSet.Contains(node.XYLeftChildPair())) || bytes.Equal(node.XYLeftChildHash, DEFAULT_HASH) {
			node.XYLeftChildHash = nil
		}
		if (node.XYRightChildHash != nil && bitStringSet.Contains(node.XYRightChildPair())) || bytes.Equal(node.XYRightChildHash, DEFAULT_HASH) {
			node.XYRightChildHash = nil
		}

		if (node.ZLeftChildHash != nil && bitStringSet.Contains(node.ZLeftChildPair())) || bytes.Equal(node.ZLeftChildHash, DEFAULT_HASH) {
			node.ZLeftChildHash = nil
		}
		if (node.ZRightChildHash != nil && bitStringSet.Contains(node.ZRightChildPair())) || bytes.Equal(node.ZRightChildHash, DEFAULT_HASH) {
			node.ZRightChildHash = nil
		}

		responseNode := &comm.Node{
			XYBitString:       node.XYBitString,
			XYBitStringLen:    uint32(node.XYBitStringLen),
			ZBitString:        uint32(node.ZBitString),
			ZBitStringLen:     uint32(node.ZBitStringLen),
			NeighborHash:      node.NeighborHash,
			XYLeftChildHash:   node.XYLeftChildHash,
			XYRightChildHash:  node.XYRightChildHash,
			ZLeftChildHash:    node.ZLeftChildHash,
			ZRightChildHash:   node.ZRightChildHash,
			CertificateHashes: node.CertificateHashes,
			Certificates:      [][]byte{},
		}

		responseNodes = append(responseNodes, responseNode)
	}

	// Any errors encountered by rows.Next or rows.Scan will be returned here
	err = rows.Err()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Scanning row failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "scanning row failed, check the server logs",
		})
		return
	}

	response, err := proto.Marshal(&comm.Response{
		Nodes: responseNodes,
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "Marshalling response failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "marshalling response failed, check the server logs",
		})
		return
	}

	c.Data(
		http.StatusOK,
		"application/octet-stream",
		response,
	)

}
