package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"

	"geopki/pkg/comm"
	"geopki/pkg/crypto"
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
	dbPool     *pgxpool.Pool
	privateKey *ecdsa.PrivateKey
}

func main() {
	var err error

	var listenAddress string
	var listenPort uint64

	var databaseUrl string
	var privateKeyBase64 string
	var privateKey *ecdsa.PrivateKey

	flag.StringVar(&listenAddress, "address", "0.0.0.0", "The address to listen on")
	flag.Uint64Var(&listenPort, "port", 1234, "The port to listen on")
	flag.Parse()

	if listenPort > math.MaxUint16 {
		fmt.Fprintf(os.Stderr, "Invalid port value '%d'\n", listenPort)
		os.Exit(1)
	}

	// load database url from env variable, should not show up in the history
	databaseUrl = os.Getenv("DATABASE_URL")
	if len(databaseUrl) == 0 {
		fmt.Fprintf(os.Stderr, "No 'DATABASE_URL' environment variable provided\n")
		os.Exit(2)
	}

	// load private key from env variable, should not show up in the history
	privateKeyBase64 = os.Getenv("PRIVATE_KEY")
	if len(privateKeyBase64) == 0 {
		fmt.Printf("No 'PRIVATE_KEY' (DER, then base64 encoded) environment variable provided, generate random one in memory\n")
		privateKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed generating a random private key\n")
			os.Exit(3)
		}
	} else {
		derPrivateKey, err := base64.StdEncoding.DecodeString(privateKeyBase64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed decoding the private key, must be base64 encoded\n")
			os.Exit(4)
		}

		privateKey, err = x509.ParseECPrivateKey(derPrivateKey)

		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed decoding the private key, it must be DER encoded (and then base64).\n")
			os.Exit(5)
		}
	}

	// create a connection pool
	dbPool, err := pgxpool.New(context.Background(), databaseUrl)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to create connection pool: %v\n", err)
		os.Exit(6)
	}

	// check if the connection is working
	err = dbPool.Ping(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connect to the database: %v\n", err)
		os.Exit(7)
	}

	defer dbPool.Close()

	r := gin.Default()

	// configure gin engine
	r.SetTrustedProxies(TRUSTED_PROXIES)

	// create handler environment
	env := &EndpointHandlerEnv{
		dbPool:     dbPool,
		privateKey: privateKey,
	}

	// install endpoints
	r.POST("/v1/query", env.postQuery)
	r.GET("/v1/public-key", env.getPublicKey)

	// start server
	r.Run(fmt.Sprintf("%s:%d", listenAddress, listenPort))
}

// handler for the /query endpoint
func (env *EndpointHandlerEnv) postQuery(c *gin.Context) {
	// check the content type request header
	contentTypeHeaders, ok := c.Request.Header["Content-Type"]
	if ok {
		// if the content type header is set, make sure it is exactly 'application/octet-stream'
		if len(contentTypeHeaders) > 1 || contentTypeHeaders[0] != "application/octet-stream" {

			// display an error to the user
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf(
					"Content-Type:%s is not supported. Don't set the header or use 'application/octet-stream'.",
					contentTypeHeaders[0],
				),
			})

			return
		}
	}

	includeCertificates := c.DefaultQuery("include-certificates", "none") != "none"

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

	sqlQuery := database.BuildNodeQuery(requestBitStringPairs, minAltitude, maxAltitude)

	rows, err := env.dbPool.Query(
		context.Background(),
		sqlQuery,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "node query failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "database query failed, check the server logs",
		})
		return
	}

	defer rows.Close()

	// at least allocate a capacity of 'len(bit_strings)', then let
	// the go standard libary handle growth
	nodes, rootHash, certificateStringHashes, err := database.RowsToNodesAndRootHash(rows, len(requestBitStringPairs))
	if err != nil {
		fmt.Fprintf(os.Stderr, "scanning node rows failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "scanning rows failed, check the server logs",
		})
		return
	}

	if rootHash == nil {
		fmt.Fprintf(os.Stderr, "integrity check failed, root node was not returned by the query")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "integrity check failed, root node was not returned by the query",
		})
		return
	}

	// TODO: change to persistent SMH, not on the fly-computed smhs
	smh := crypto.SignedMapHead{
		MapHead: crypto.MapHead{
			RootHash:  rootHash,
			Timestamp: 0,
		},
	}
	smh.Sign(env.privateKey)

	var certificates [][]byte
	if includeCertificates && certificateStringHashes.Cardinality() > 0 {
		sqlQuery := database.BuildCertificateQuery(certificateStringHashes.ToSlice())

		rows, err := env.dbPool.Query(
			context.Background(),
			sqlQuery,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "certificate query failed: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "database query failed, check the server logs",
			})
			return
		}

		defer rows.Close()

		// at least allocate a capacity of 'len(bit_strings)', then let
		// the go standard libary handle growth
		certificates, err = database.RowsToCertificates(rows, certificateStringHashes.Cardinality())
		if err != nil {
			fmt.Fprintf(os.Stderr, "scanning certificate rows failed: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "scanning rows failed, check the server logs",
			})
			return
		}
	}

	response, err := proto.Marshal(&comm.Response{
		SignedMapHead: smh.Proto(),
		Nodes:         nodes,

		Certificates: certificates,
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

// handler for the /public-key endpoint
func (env *EndpointHandlerEnv) getPublicKey(c *gin.Context) {
	// check the content type request header
	contentTypeHeaders, ok := c.Request.Header["Content-Type"]
	if ok {
		// if the content type header is set, make sure it is exactly 'application/octet-stream'
		if len(contentTypeHeaders) > 1 || contentTypeHeaders[0] != "application/octet-stream" {

			// display an error to the user
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf(
					"Content-Type:%s is not supported. Don't set the header or use 'application/octet-stream'.",
					contentTypeHeaders[0],
				),
			})

			return
		}
	}

	publicKey, err := x509.MarshalPKIXPublicKey(&env.privateKey.PublicKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed marshalling public key: %v\n", err)

		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Failed marshalling public key",
		})
	}

	c.Data(
		http.StatusOK,
		"application/octet-stream",
		publicKey,
	)
}
