package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	"geopki/pkg/comm"
	"geopki/pkg/database"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"
)

// 'constants', arrays can't be set to 'const' though!
var TRUSTED_PROXIES = []string{"localhost"}

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
	query, err := io.ReadAll(c.Request.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Reading request body failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	requestBitStringPairs, minAltitude, maxAltitude, err := comm.ParseQuery(query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Reading request body failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	sqlQuery := database.BuildQuery(requestBitStringPairs, minAltitude, maxAltitude)

	rows, err := env.dbPool.Query(
		context.Background(),
		sqlQuery,
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
	nodes, err := database.RowsToNodes(rows, len(requestBitStringPairs))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Scanning row failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "scanning row failed, check the server logs",
		})
		return
	}

	response, err := proto.Marshal(&comm.Response{
		Nodes: nodes,
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
