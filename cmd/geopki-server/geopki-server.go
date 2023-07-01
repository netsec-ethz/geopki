package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
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
	"geopki/pkg/geometry"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/pprof"
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

	DATABASE_STATE_KEY_DIRTY = "dirty"
)

var TRUSTED_PROXIES = []string{"localhost"}

type EndpointHandlerEnv struct {
	dbPool *pgxpool.Pool
	// key used to sign cryptographic statements
	privateKey *ecdsa.PrivateKey
	// key for inserting certificates
	certificateInsertionKeyHash []byte

	// client for accessing the consistency tree
	consistencyClient *crypto.ConsistencyTreeClient

	// lock for accessing cached data or state data, not required for values defined
	// above since they never change
	sharedDataLock sync.RWMutex
	// lock for updating the DB
	updateLock sync.Mutex

	// caches the most recent SMH value
	currentSignedMapHead *crypto.SignedMapHead
	// caches the most recent SCH value
	currentSignedConsistencyHead *crypto.SignedConsistencyHead
	// caches the inclusion proof for the latest SCH value
	schInclusionProof []byte

	// persistent state
	// whether the database contents are dirty and no valid data can be returned
	isDirty bool
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

	// load presistent state

	tx, err := dbPool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.Serializable,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to start transaction: %v\n", err)
		os.Exit(10)
	}

	dirty, err := database.QueryState(DATABASE_STATE_KEY_DIRTY, tx, ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to query persistent database state '%s': %v\n", DATABASE_STATE_KEY_DIRTY, err)
		os.Exit(11)
	}

	err = tx.Commit(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to commit transaction: %v\n", err)
		os.Exit(12)
	}

	// ensure the existence of a consistency tree service
	consistencyClient, err := crypto.NewConsistencyTreeClient(trillianAddress, consistencyLogId, privateKey, MAXIMUM_MERGE_DELAY)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to create consistency client: %v\n", err)
		os.Exit(13)
	}

	sch, err := consistencyClient.LatestSignedConsistencyHead(ctx)
	if err != nil {

		err2 := consistencyClient.InitializeLog(ctx)

		if err2 != nil {
			fmt.Fprintf(os.Stderr, "unable to obtain latest consistency head: %v\n", err)
			fmt.Fprintf(os.Stderr, "unable to initialize log server: %v\n", err2)
			os.Exit(14)
		}
	}

	// if the log was newly initialized, add the current SMH
	if err != nil || sch.Size == 0 {
		// query the current root hash and db state
		tx, err := dbPool.BeginTx(ctx, pgx.TxOptions{
			IsoLevel: pgx.Serializable,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to start transaction: %v\n", err)
			os.Exit(15)
		}

		rootHash, err := database.QueryRootHash(tx, ctx)
		if err != nil {
			// root hash does not exist -> is sparse
			rootHash = crypto.DEFAULT_HASH
		}

		err = tx.Commit(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to commit transaction: %v\n", err)
			os.Exit(16)
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
			os.Exit(17)
		}
	}

	// cache the current smh
	smh, err := consistencyClient.LatestSignedMapHead(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to obtain latest signed map head: %v\n", err)
		os.Exit(18)
	}

	if !smh.Verify(&privateKey.PublicKey) {
		fmt.Fprintf(os.Stderr, "cannot verify the signature on the latest SMH, did the private key change?\n")
		os.Exit(19)
	}

	proof, err := consistencyClient.ProveSignedMapHeadInclusion(context.Background(), sch.Size, smh)
	if err != nil {
		fmt.Fprintf(os.Stderr, "obtaining a proof of inclusion for consistency tree failed: %v\n", err)
		os.Exit(20)
	}

	inclusionProof, err := proto.Marshal(proof)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed marshaling inclusion proof: %v\n", err)
		os.Exit(21)
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
		schInclusionProof:            inclusionProof,

		isDirty: dirty == "true",
	}

	// setup web server
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	pprof.Register(r)

	// configure gin engine
	r.SetTrustedProxies(TRUSTED_PROXIES)

	// setup middlewares
	r.Use(gin.Recovery())
	// do not use compression, lowers the throughput
	// r.Use(gzip.Gzip(gzip.DefaultCompression))
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
	r.POST("/v1/drop-indices", env.getDropIndices)
	r.POST("/v1/finish-partial", env.getFinishPartial)

	// install demo endpoint
	r.Static("/demo", "./demo/geopki-web-client")

	// start server
	r.Run(fmt.Sprintf("%s:%d", listenAddress, listenPort))
}

// handler for the /query endpoint
func (env *EndpointHandlerEnv) postQuery(c *gin.Context) {

	// acquire read lock on cache for the duration of the query to guarantee
	// the cached data is consistent with the data retrieved from the db
	env.sharedDataLock.RLock()
	defer env.sharedDataLock.RUnlock()

	if env.isDirty {
		// display an error to the user
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "The database is being updated, try to query later again",
		})

		return
	}

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

	// allocate slice with capacity of 'len(bit_strings)' and let go handle slice growth
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

		// allocate slice with capacity of 'certificateStringHashes.Cardinality()', let go handle slice growth
		certificates, err = database.RowsToCertificates(rows, certificateStringHashes.Cardinality())
		if err != nil {
			fmt.Fprintf(os.Stderr, "scanning certificate rows failed: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "scanning rows failed, check the server logs",
			})
			return
		}
	}

	sch := env.currentSignedConsistencyHead
	smh := env.currentSignedMapHead
	inclusionProof := env.schInclusionProof

	response, err := proto.Marshal(&comm.Response{
		SignedConsistencyHead: sch.Proto(),
		SignedMapHead:         smh.Proto(),
		InclusionProof:        inclusionProof,
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
	env.sharedDataLock.RLock()
	sch := env.currentSignedConsistencyHead
	env.sharedDataLock.RUnlock()

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
	env.sharedDataLock.RLock()
	smh := env.currentSignedMapHead
	env.sharedDataLock.RUnlock()

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
	if !env.receivedValidInsertionKey(c) {
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

	// by default the hashes are updated, can be turned off for partial insertions,
	// especially the initial insertion where hashes are computed over and over again otherwise
	isPartialUpdate := c.DefaultQuery("is-partial", "none") != "none"

	// read request body
	zr, err := gzip.NewReader(c.Request.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating gzip reader failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "creating gzip reader, check the server logs",
		})
		return
	}

	body, err := io.ReadAll(zr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading gzipped request body failed: %v\n", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	if err := zr.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "closing gzip reader failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "closing gzip reader, check the server logs",
		})
		return
	}

	var certificates []*crypto.GeoCertificate
	err = json.Unmarshal(body, &certificates)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf(
				"supplied invalid certificates, %v",
				err,
			),
		})
		return
	}

	for _, certificate := range certificates {
		marshaledCert, err := json.Marshal(certificate)
		if err != nil {
			fmt.Fprintf(os.Stderr, "marshaling certificate failed: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "marshaling certificates failed, check the server logs",
			})
			return
		}
		certificate.MarshaledCert = marshaledCert
	}

	didLock := env.updateLock.TryLock()
	if !didLock {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "update is already in progress, try again later",
		})
		return
	}

	// unlock after returning
	defer env.updateLock.Unlock()

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

	defer tx.Rollback(c.Request.Context())

	err = database.AddNewCertificates(
		certificates,
		F_GROW,
		tx,
		// TODO: set the set of covered log servers for instance by storing that in the db and retrieving it here
		[]crypto.CTLogServer{},
		// update hashes if it is *not* a partial update
		!isPartialUpdate,
		c.Request.Context(),
	)

	if err != nil {
		fmt.Fprintf(os.Stderr, "failed updating SMT: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed updating SMT, check the server logs",
		})
		return
	}

	if isPartialUpdate {
		// if set to false, set to true, noop if already true
		_, err := database.UpdateState(DATABASE_STATE_KEY_DIRTY, "false", "true", tx, c.Request.Context())
		if err != nil {
			fmt.Fprintf(os.Stderr, "updating state '%s' failed: %v\n", DATABASE_STATE_KEY_DIRTY, err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "updating internal state failed, check the server logs",
			})
			return
		}

		// before transaction is commited, acquire lock on shared data
		env.sharedDataLock.Lock()
		defer env.sharedDataLock.Unlock()
	}

	err = tx.Commit(c.Request.Context())
	if err != nil {
		fmt.Fprintf(os.Stderr, "commiting transaction failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "commiting transaction failed, check the server logs",
		})
		return
	}

	if isPartialUpdate {
		// update was persisted and we acquired a lock, update the shared state
		env.isDirty = true
	}

	c.Data(
		http.StatusOK,
		"application/json",
		[]byte("{\"success\":true}"),
	)

	fmt.Printf("counter: %d\n", geometry.Counter)
}

func (env *EndpointHandlerEnv) getDropIndices(c *gin.Context) {
	if !env.receivedValidInsertionKey(c) {
		return
	}

	didLock := env.updateLock.TryLock()
	if !didLock {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Update is already in progress, try again later",
		})
		return
	}

	// unlock after returning
	defer env.updateLock.Unlock()

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

	defer tx.Rollback(c.Request.Context())

	// drop indices for faster insertion
	_, err = tx.Exec(
		c.Request.Context(),
		// nodes table
		"DROP INDEX IF EXISTS bit_string_bit_idx;"+
			"DROP INDEX IF EXISTS bit_string_len;"+
			"DROP INDEX IF EXISTS bit_string_integer_idx;"+
			// certificates table
			"DROP INDEX IF EXISTS certificate_hash;"+
			"DROP INDEX IF EXISTS certificate_not_valid_after;",
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dropping indices failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "dropping indices failed",
		})
		return
	}

	// update persistent state to dirty
	_, err = database.UpdateState(DATABASE_STATE_KEY_DIRTY, "false", "true", tx, c.Request.Context())
	if err != nil {
		fmt.Fprintf(os.Stderr, "updating state '%s' failed: %v\n", DATABASE_STATE_KEY_DIRTY, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "updating internal state failed, check the server logs",
		})
		return
	}

	// before the data appears in the db, acquire a lock on the cached data
	// otherwise a reader might observe inconsistent data
	env.sharedDataLock.Lock()
	defer env.sharedDataLock.Unlock()

	err = tx.Commit(c.Request.Context())
	if err != nil {
		fmt.Fprintf(os.Stderr, "commiting transaction failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "commiting transaction failed, check the server logs",
		})
		return
	}

	// after sucessful commitment, update shared state
	// we already acquired the lock
	env.isDirty = true

	c.Data(
		http.StatusOK,
		"application/json",
		[]byte("{\"success\":true}"),
	)
}

func (env *EndpointHandlerEnv) getFinishPartial(c *gin.Context) {
	if !env.receivedValidInsertionKey(c) {
		return
	}

	didLock := env.updateLock.TryLock()
	if !didLock {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Update is already in progress, try again later",
		})
		return
	}

	// unlock after returning
	defer env.updateLock.Unlock()

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

	defer tx.Rollback(c.Request.Context())

	// re-compute all hashes
	_, err = tx.Exec(c.Request.Context(), "SELECT compute_hashes()")
	if err != nil {
		fmt.Fprintf(os.Stderr, "updating hashes failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "updating hashes failed, check the server logs",
		})
		return
	}

	// re-create indices
	_, err = tx.Exec(
		c.Request.Context(),
		// nodes table
		"CREATE UNIQUE INDEX IF NOT EXISTS bit_string_bit_idx ON nodes USING btree (bit_string_51 ASC NULLS LAST, bit_string_15 ASC NULLS LAST);"+
			"CREATE INDEX IF NOT EXISTS bit_string_len ON nodes (LENGTH(bit_string_51), LENGTH(bit_string_15));"+
			"CREATE INDEX IF NOT EXISTS bit_string_integer_idx ON nodes USING btree (bit_string_51_int ASC NULLS LAST);"+
			"ALTER TABLE IF EXISTS nodes CLUSTER ON bit_string_integer_idx;"+
			"CLUSTER nodes USING bit_string_integer_idx;"+
			"VACUUM FULL nodes;"+
			// certificates table
			"CREATE UNIQUE INDEX IF NOT EXISTS certificate_hash ON certificates USING hash (certificate_hash);"+
			"CREATE INDEX IF NOT EXISTS certificate_not_valid_after ON certificates USING btree (not_valid_after);"+
			"ALTER TABLE IF EXISTS certificates CLUSTER ON certificate_not_valid_after;"+
			"CLUSTER certificates USING certificate_not_valid_after;",
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating indices and constraints failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "creating indices and constraints failed",
		})
		return
	}

	smh, err := env.createNewSMH(tx, c.Request.Context())
	if err != nil {
		fmt.Fprintf(os.Stderr, "updating SMH failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "updating SMH failed, check the server logs",
		})
		return
	}

	// update persistent state, no longer dirty
	_, err = database.UpdateState(DATABASE_STATE_KEY_DIRTY, "true", "false", tx, c.Request.Context())
	if err != nil {
		fmt.Fprintf(os.Stderr, "updating state '%s' failed: %v\n", DATABASE_STATE_KEY_DIRTY, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "updating internal state failed, check the server logs",
		})
		return
	}

	// before the data appears in the db, acquire a lock on the cached data
	// otherwise a reader might observe inconsistent data
	env.sharedDataLock.Lock()
	defer env.sharedDataLock.Unlock()

	err = tx.Commit(c.Request.Context())
	if err != nil {
		fmt.Fprintf(os.Stderr, "commiting transaction failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "commiting transaction failed, check the server logs",
		})
		return
	}

	// after sucessful commitment, update shared state
	// we already acquired the lock
	env.isDirty = false

	// update sch if transaction committed
	sch, err := env.updateSCH(smh, c.Request.Context())
	if err != nil {
		fmt.Fprintf(os.Stderr, "updating SCH failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "updating SCH failed, check the server logs",
		})
		return
	}

	c.Data(
		http.StatusOK,
		"application/json",
		[]byte(fmt.Sprintf("{\"success\":true, \"new_tree_size\":%d}", sch.Size)),
	)
}

func (env *EndpointHandlerEnv) receivedValidInsertionKey(c *gin.Context) bool {
	key := c.DefaultQuery("key", "???")
	keyHash := sha256.Sum256([]byte(key))

	// compare hashes, avoids timing side channel since the strings are of the same length
	if !bytes.Equal(keyHash[:], env.certificateInsertionKeyHash) {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "invalid key",
		})
		return false
	}

	return true
}

func (env *EndpointHandlerEnv) createNewSMH(
	tx pgx.Tx,
	ctx context.Context,
) (
	*crypto.SignedMapHead,
	error,
) {

	rootHash, err := database.QueryRootHash(tx, ctx)
	if err != nil {
		return nil, fmt.Errorf("could not fetch root hash: %v", err)
	}

	// create new SMH *WITHOUT* signature
	smh := &crypto.SignedMapHead{
		MapHead: crypto.MapHead{
			RootHash:  rootHash,
			Timestamp: uint64(time.Now().UnixNano()),
			// TODO: set the set of covered log servers for instance by storing that in the db and retrieving it here
			CoveredCTLogServers: []crypto.CTLogServer{},
		},
	}

	// sign SMH
	smh.Sign(env.privateKey)

	return smh, nil
}

// creates a new sch and updates the cached values in env, the caller must ensure a lock is held
func (env *EndpointHandlerEnv) updateSCH(
	smh *crypto.SignedMapHead,
	ctx context.Context,
) (
	*crypto.SignedConsistencyHead,
	error,
) {
	// insert it into the consistency tree
	sch, err := env.consistencyClient.AppendSignedMapHead(ctx, smh)
	if err != nil {
		return nil, fmt.Errorf("failed updating the consistency tree: %v", err)
	}

	proof, err := env.consistencyClient.ProveSignedMapHeadInclusion(ctx, sch.Size, smh)
	if err != nil {
		return nil, fmt.Errorf("obtaining a proof of inclusion for consistency tree failed: %v", err)
	}

	marshaledProof, err := proto.Marshal(proof)
	if err != nil {
		return nil, fmt.Errorf("failed marshaling inclusion proof: %v", err)
	}

	// update cached smh and sch, acquire lock to ensure all reads are consistent
	env.currentSignedMapHead = smh
	env.currentSignedConsistencyHead = sch
	env.schInclusionProof = marshaledProof

	return sch, nil
}
