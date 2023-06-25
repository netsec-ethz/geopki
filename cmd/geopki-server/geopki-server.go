package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"geopki/pkg/comm"
	"geopki/pkg/crypto"
	"geopki/pkg/database"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"
)

const (
	// maximum merge delay in seconds
	MAXIMUM_MERGE_DELAY = 5
	// the f factor for new certificates
	F_GROW = 0.1
)

type EndpointHandlerEnv struct {
	dbPool *pgxpool.Pool
	// key used to sign cryptographic statements
	privateKey *ecdsa.PrivateKey
	// key for inserting certificates
	certificateInsertionKeyHash []byte

	// client for accessing the consistency tree
	consistencyClient *crypto.ConsistencyTreeClient

	// lock for accessing cached SMH / SCH
	cacheLock sync.RWMutex
	// lock for updating the DB
	updateLock sync.Mutex

	// caches the most recent SMH value
	currentSignedMapHead *crypto.SignedMapHead
	// caches the most recent SCH value
	currentSignedConsistencyHead *crypto.SignedConsistencyHead
}

func main() {
	var err error

	var listenAddress string
	var listenPort uint64

	var trillianAddress string
	var consistencyLogId int64

	var privateKeyBase64 string
	var privateKey *ecdsa.PrivateKey

	var certificateInsertionKey string
	var certificateInsertionKeyHash [32]byte

	var databaseUrl string

	flag.StringVar(&listenAddress, "address", "0.0.0.0", "The address to listen on")
	flag.Uint64Var(&listenPort, "port", 1234, "The port to listen on")

	// run a trillian instance
	// for development, docker setup described at https://github.com/google/trillian/tree/v1.5.2/examples/deployment works well
	flag.StringVar(&trillianAddress, "trillian-address", "localhost:8090", "The address of the trillian server serving the consistency tree")
	flag.Int64Var(&consistencyLogId, "clog-id", 1, "The log id of the consistency tree on the trillian server")
	flag.Parse()

	ctx := context.Background()

	if listenPort > math.MaxUint16 {
		fmt.Fprintf(os.Stderr, "invalid port value '%d'\n", listenPort)
		os.Exit(1)
	}

	// load private key from env variable, should not show up in the history
	privateKeyBase64 = os.Getenv("PRIVATE_KEY")
	if len(privateKeyBase64) == 0 {
		fmt.Printf("No 'PRIVATE_KEY' (DER, then base64 encoded) environment variable provided, generate random one in memory\n")
		privateKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

		if err != nil {
			fmt.Fprintf(os.Stderr, "failed generating a random private key\n")
			os.Exit(2)
		}

		marshaledPrivateKey, err := x509.MarshalECPrivateKey(privateKey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed marshaling random private key\n")
			os.Exit(3)
		}

		fmt.Printf("Using the following private key: PRIVATE_KEY=%s\n", base64.StdEncoding.EncodeToString(marshaledPrivateKey))

	} else {
		derPrivateKey, err := base64.StdEncoding.DecodeString(privateKeyBase64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed decoding the private key, must be base64 encoded\n")
			os.Exit(4)
		}

		privateKey, err = x509.ParseECPrivateKey(derPrivateKey)

		if err != nil {
			fmt.Fprintf(os.Stderr, "failed decoding the private key, it must be DER encoded (and then base64).\n")
			os.Exit(5)
		}
	}

	// load certificate insertion key from env variable, should not show up in the history
	certificateInsertionKey = os.Getenv("CERT_INSERT_KEY")
	if len(certificateInsertionKey) == 0 {
		fmt.Fprintf(os.Stderr, "no 'CERT_INSERT_KEY' environment variable provided\n")
		os.Exit(6)
	}
	certificateInsertionKeyHash = sha256.Sum256([]byte(certificateInsertionKey))

	// load database url from env variable, should not show up in the history
	databaseUrl = os.Getenv("DATABASE_URL")
	if len(databaseUrl) == 0 {
		fmt.Fprintf(os.Stderr, "no 'DATABASE_URL' environment variable provided\n")
		os.Exit(7)
	}

	// create a connection pool
	dbPool, err := pgxpool.New(ctx, databaseUrl)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to create connection pool: %v\n", err)
		os.Exit(8)
	}

	// check if the connection is working
	err = dbPool.Ping(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to connect to the database: %v\n", err)
		os.Exit(9)
	}

	defer dbPool.Close()

	// ensure the existence of a consistency tree service
	consistencyClient, err := crypto.NewConsistencyTreeClient(trillianAddress, consistencyLogId, privateKey, MAXIMUM_MERGE_DELAY)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to create consistency client: %v\n", err)
		os.Exit(10)
	}

	sch, err := consistencyClient.LatestSignedConsistencyHead(ctx)
	if err != nil {

		err2 := consistencyClient.InitializeLog(ctx)

		if err2 != nil {
			fmt.Fprintf(os.Stderr, "unable to obtain latest consistency head: %v\n", err)
			fmt.Fprintf(os.Stderr, "unable to initialize log server: %v\n", err2)
			os.Exit(11)
		}
	}

	// if the log was newly initialized, add the current SMH
	if err != nil || sch.Size == 0 {
		// query the current root hash
		tx, err := dbPool.BeginTx(ctx, pgx.TxOptions{
			IsoLevel: pgx.Serializable,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to start transaction: %v\n", err)
			os.Exit(12)
		}

		rootHash, err := database.QueryRootHash(tx, ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to query root hash: %v\n", err)
			os.Exit(13)
		}

		err = tx.Commit(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to commit transaction: %v\n", err)
			os.Exit(14)
		}

		// create new SMH
		smh := &crypto.SignedMapHead{
			MapHead: crypto.MapHead{
				RootHash:  rootHash,
				Timestamp: uint64(time.Now().UnixNano()),
				// TODO: set the set of covered log servers for instance by storing that in the db and retrieving it here
				CoveredCTLogServers: []crypto.CTLogServer{},
			},
		}
		smh.Sign(privateKey)

		// insert it into the consistency tree
		sch, err = consistencyClient.AppendSignedMapHead(ctx, smh)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to append new SMH: %v\n", err)
			os.Exit(15)
		}
	}

	// cache the current smh
	smh, err := consistencyClient.LatestSignedMapHead(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to obtain latest signed map head: %v\n", err)
		os.Exit(16)
	}

	if !smh.Verify(&privateKey.PublicKey) {
		fmt.Fprintf(os.Stderr, "cannot verify the signature on the latest SMH, did the private key change?\n")
		os.Exit(17)
	}

	fmt.Printf("Serving data with SMH:\n%s\n\n", smh.String())

	// create handler environment for shared data
	env := &EndpointHandlerEnv{
		dbPool:                      dbPool,
		privateKey:                  privateKey,
		certificateInsertionKeyHash: certificateInsertionKeyHash[:],

		consistencyClient: consistencyClient,

		currentSignedMapHead:         smh,
		currentSignedConsistencyHead: sch,
	}

	// setup web server
	r := gin.Default()

	// configure gin engine

	// setup middlewares
	r.Use(gzip.Gzip(gzip.BestCompression))
	r.Use(cors.New(cors.Config{
		AllowAllOrigins:  true,
		AllowMethods:     []string{"GET", "POST", "HEAD"},
		AllowHeaders:     []string{"Content-Length", "Content-Type"},
		AllowCredentials: false,
	}))

	// install endpoints
	r.POST("/v1/query", env.postQuery)
	r.GET("/v1/certificates", env.getCertificates)
	r.GET("/v1/public-key", env.getPublicKey)
	r.GET("/v1/get-sch", env.getSignedConsistencyHead)
	r.GET("/v1/get-smh", env.getSignedMapHead)
	r.GET("/v1/get-sch-consistency", env.getSignedConsistencyHeadConsistency)
	r.GET("/v1/get-proof-by-hash", env.getProofByHash)
	r.GET("/v1/get-entries", env.getEntries)
	r.GET("/v1/get-entry-and-proof", env.getEntryAndProof)
	r.POST("/v1/insert", env.postInsert)

	// install demo endpoint
	r.Static("/demo", "./demo/geopki-web-client")

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
		fmt.Fprintf(os.Stderr, "reading request body failed: %v\n", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	requestBitStringPairs, minAltitude, maxAltitude, err := comm.ParseQuery(query)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	sqlQuery := database.BuildNodeQuery(requestBitStringPairs, minAltitude, maxAltitude)

	rows, err := env.dbPool.Query(
		c.Request.Context(),
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

	var certificates [][]byte
	if includeCertificates && certificateStringHashes.Cardinality() > 0 {
		sqlQuery, err := database.BuildCertificateQuery(certificateStringHashes.ToSlice())
		if err != nil {
			fmt.Fprintf(os.Stderr, "building certificate query failed: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "building database query failed, check the server logs",
			})
			return
		}

		rows, err := env.dbPool.Query(
			c.Request.Context(),
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

	env.cacheLock.RLock()
	sch := env.currentSignedConsistencyHead
	smh := env.currentSignedMapHead
	env.cacheLock.RUnlock()

	proof, err := env.consistencyClient.ProveSignedMapHeadInclusion(c.Request.Context(), sch.Size, smh)
	if err != nil {
		fmt.Fprintf(os.Stderr, "obtaining a proof of inclusion for consistency tree failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "obtaining a proof of inclusion for consistency tree failed, check the server logs",
		})
		return
	}

	marshaledProof, err := proto.Marshal(proof)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed marshaling inclusion proof: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed marshaling inclusion proof, check the server logs",
		})
		return
	}

	response, err := proto.Marshal(&comm.Response{
		SignedConsistencyHead: sch.Proto(),
		SignedMapHead:         smh.Proto(),
		InclusionProof:        marshaledProof,
		Nodes:                 nodes,

		Certificates: certificates,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed marshaling response: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed marshaling response, check the server logs",
		})
		return
	}

	c.Data(
		http.StatusOK,
		"application/octet-stream",
		response,
	)

}

// handler for the /certificates endpoint
func (env *EndpointHandlerEnv) getCertificates(c *gin.Context) {

	certificateStringHashes, nonEmpty := c.GetQueryArray("hash")
	if !nonEmpty {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "provide at least one hash using the 'hash' parameter",
		})
		return
	}

	var certificates [][]byte
	sqlQuery, err := database.BuildCertificateQuery(certificateStringHashes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "building certificate query failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "building database query failed, check the server logs",
		})
		return
	}

	rows, err := env.dbPool.Query(
		c.Request.Context(),
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
	certificates, err = database.RowsToCertificates(rows, len(certificateStringHashes))
	if err != nil {
		fmt.Fprintf(os.Stderr, "scanning certificate rows failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "scanning rows failed, check the server logs",
		})
		return
	}

	response, err := proto.Marshal(&comm.Response{
		Certificates: certificates,
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "failed marshaling response: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed marshaling response, check the server logs",
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
		fmt.Fprintf(os.Stderr, "failed marshaling public key: %v\n", err)

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed marshaling public key",
		})
		return
	}

	c.Data(
		http.StatusOK,
		"application/octet-stream",
		publicKey,
	)
}

func (env *EndpointHandlerEnv) getSignedConsistencyHead(c *gin.Context) {
	env.cacheLock.RLock()
	sch := env.currentSignedConsistencyHead
	env.cacheLock.RUnlock()

	response, err := proto.Marshal(sch.Proto())
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed marshaling response: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed marshaling response, check the server logs",
		})
		return
	}

	c.Data(
		http.StatusOK,
		"application/octet-stream",
		response,
	)
}

func (env *EndpointHandlerEnv) getSignedMapHead(c *gin.Context) {
	env.cacheLock.RLock()
	smh := env.currentSignedMapHead
	env.cacheLock.RUnlock()

	response, err := proto.Marshal(smh.Proto())
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed marshaling response: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed marshaling response, check the server logs",
		})
		return
	}

	c.Data(
		http.StatusOK,
		"application/octet-stream",
		response,
	)
}

func (env *EndpointHandlerEnv) getSignedConsistencyHeadConsistency(c *gin.Context) {

	treeSize1Str := c.DefaultQuery("first", "abc")
	treeSize2Str := c.DefaultQuery("second", "abc")

	treeSize1, err := strconv.ParseUint(treeSize1Str, 2, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "received invalid tree size for argument 'first'",
		})
		return
	}

	treeSize2, err := strconv.ParseUint(treeSize2Str, 2, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "received invalid tree size for argument 'second'",
		})
		return
	}

	if treeSize1 == treeSize2 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "the two tree sizes cannot be the same",
		})
		return
	}

	proof, err := env.consistencyClient.ConsistencyProof(c.Request.Context(), treeSize1, treeSize2)
	if err != nil {
		fmt.Fprintf(os.Stderr, "obtaining consistency proof failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "retrieving consistency proof failed, check the server logs",
		})
		return
	}

	response, err := proto.Marshal(proof)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed marshaling response: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed marshaling response, check the server logs",
		})
		return
	}

	c.Data(
		http.StatusOK,
		"application/octet-stream",
		response,
	)
}

func (env *EndpointHandlerEnv) getProofByHash(c *gin.Context) {

	hashBase64 := c.DefaultQuery("hash", "#")
	treeSizeStr := c.DefaultQuery("tree_size", "abc")

	treeSize, err := strconv.ParseUint(treeSizeStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "received invalid tree size for argument 'tree_size'",
		})
		return
	}

	hash, err := base64.RawURLEncoding.DecodeString(hashBase64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	proof, err := env.consistencyClient.ProveSignedMapHeadHashInclusion(c.Request.Context(), treeSize, hash)
	if err != nil {
		fmt.Fprintf(os.Stderr, "obtaining a proof of inclusion for consistency tree failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "obtaining a proof of inclusion for consistency tree failed, check the server logs",
		})
		return
	}

	response, err := proto.Marshal(proof)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed marshaling response: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed marshaling response, check the server logs",
		})
		return
	}

	c.Data(
		http.StatusOK,
		"application/octet-stream",
		response,
	)
}

func (env *EndpointHandlerEnv) getEntries(c *gin.Context) {
	startStr := c.DefaultQuery("start", "abc")
	endStr := c.DefaultQuery("end", "abc")

	start, err := strconv.ParseUint(startStr, 2, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "received invalid index for argument 'start'",
		})
		return
	}

	end, err := strconv.ParseUint(endStr, 2, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "received invalid index for argument 'end'",
		})
		return
	}

	if start > end {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "the 'end' value must be greater than or equal to 'start'",
		})
		return
	}

	entries, err := env.consistencyClient.GetEntries(c.Request.Context(), start, end)
	if err != nil {
		fmt.Fprintf(os.Stderr, "obtaining entries failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "obtaining entries failed, check the server logs",
		})
		return
	}

	response, err := proto.Marshal(entries)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed marshaling response: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed marshaling response, check the server logs",
		})
		return
	}

	c.Data(
		http.StatusOK,
		"application/octet-stream",
		response,
	)
}

func (env *EndpointHandlerEnv) getEntryAndProof(c *gin.Context) {
	leafIndexStr := c.DefaultQuery("leaf_index", "abc")
	treeSizeStr := c.DefaultQuery("tree_size", "abc")

	leafIndex, err := strconv.ParseUint(leafIndexStr, 2, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "received invalid index for argument 'leaf_index'",
		})
		return
	}

	treeSize, err := strconv.ParseUint(treeSizeStr, 2, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "received invalid tree size for argument 'tree_size'",
		})
		return
	}

	if leafIndex >= treeSize {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "the 'leaf_index' value must be strictly greater than 'tree_size'",
		})
		return
	}

	entryAndProof, err := env.consistencyClient.GetEntryAndProof(c.Request.Context(), treeSize, leafIndex)
	if err != nil {
		fmt.Fprintf(os.Stderr, "obtaining entry and proof failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "obtaining entry and proof failed, check the server logs",
		})
		return
	}

	response, err := proto.Marshal(entryAndProof)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed marshaling response: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed marshaling response, check the server logs",
		})
		return
	}

	c.Data(
		http.StatusOK,
		"application/octet-stream",
		response,
	)
}

func (env *EndpointHandlerEnv) postInsert(c *gin.Context) {
	key := c.DefaultQuery("key", "???")
	keyHash := sha256.Sum256([]byte(key))

	// compare hashes, avoids timing side channel since the strings are of the same length
	if !bytes.Equal(keyHash[:], env.certificateInsertionKeyHash) {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "invalid key",
		})
		return
	}

	// check the content type request header
	contentTypeHeaders, ok := c.Request.Header["Content-Type"]
	if ok {
		// if the content type header is set, make sure it is exactly 'application/json'
		if len(contentTypeHeaders) > 1 || contentTypeHeaders[0] != "application/json" {

			// display an error to the user
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf(
					"Content-Type:%s is not supported. Don't set the header or use 'application/json'.",
					contentTypeHeaders[0],
				),
			})

			return
		}
	}

	// read request body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading request body failed: %v\n", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	cert, err := crypto.UnmarshalGeoCertificate(body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf(
				"Supplied invalid certificate, %v",
				err,
			),
		})
		return
	}

	didLock := env.updateLock.TryLock()
	if !didLock {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Update is already in progress, try again later",
		})
		return
	}

	tx, err := env.dbPool.BeginTx(c.Request.Context(), pgx.TxOptions{
		IsoLevel: pgx.Serializable,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "starting transaction failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "starting transaction failed, check the server logs",
		})
		return
	}

	smh, err := database.UpdateTree(
		[]*crypto.GeoCertificate{cert},
		F_GROW,
		time.Now(),
		tx,
		// TODO: set the set of covered log servers for instance by storing that in the db and retrieving it here
		[]crypto.CTLogServer{},
		c.Request.Context(),
	)
	defer tx.Rollback(c.Request.Context())

	if err != nil {
		fmt.Fprintf(os.Stderr, "failed updating SMT: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed updating SMT, check the server logs",
		})
		return
	}

	err = tx.Commit(c.Request.Context())
	if err != nil {
		fmt.Fprintf(os.Stderr, "commiting transaction failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "commiting transaction failed, check the server logs",
		})
		return
	}

	// sign SMH
	smh.Sign(env.privateKey)

	// insert it into the consistency tree
	sch, err := env.consistencyClient.AppendSignedMapHead(c.Request.Context(), smh)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed updating the consistency tree: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "updating consistency tree failed, check the server logs",
		})
		return
	}

	// update cached smh and sch, acquire lock to ensure all reads are consistent
	env.cacheLock.Lock()
	env.currentSignedMapHead = smh
	env.currentSignedConsistencyHead = sch
	env.cacheLock.Unlock()
	env.updateLock.Unlock()

	c.Data(
		http.StatusOK,
		"application/json",
		[]byte(fmt.Sprintf("{\"success\":true, \"new_tree_size\":%d}", sch.Size)),
	)
}
