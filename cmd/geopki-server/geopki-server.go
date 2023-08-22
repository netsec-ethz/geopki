package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"flag"
	"fmt"
	"math"
	"os"
	"time"

	server "geopki/internal/geopki-server"
	"geopki/pkg/crypto"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"
)

func main() {
	var err error

	// CLI argument: the address the HTTP server should bind to
	var listenAddress string
	// CLI argument: the port the HTTP server should bind to
	var listenPort uint64

	// CLI argument: the address of the trillian server
	var trillianAddress string
	// CLI argument: the trillian tree id used for the consistency tree
	var consistencyLogId int64

	// optional ENV variable: the base64 encoded private key used to sign statements
	var privateKeyBase64 string
	// the parsed and initialized private key
	var privateKey *ecdsa.PrivateKey

	// ENV variable: some secret required to ingest new certificates or trigger a release
	var certificateInsertionKey string
	// the SHA256 hash of 'certificateInsertionKey'
	var certificateInsertionKeyHash [32]byte

	// ENV variable: the postgres database url to connect to
	var databaseUrl string

	// parse the input arguments
	flag.StringVar(&listenAddress, "address", "0.0.0.0", "The address to listen on")
	flag.Uint64Var(&listenPort, "port", 1234, "The port to listen on")

	// run a trillian instance
	// for development, docker setup described at https://github.com/google/trillian/tree/v1.5.2/examples/deployment works well
	flag.StringVar(&trillianAddress, "trillian-address", "localhost:8090", "The address of the trillian server serving the consistency tree")
	flag.Int64Var(&consistencyLogId, "clog-id", 1, "The log id of the consistency tree on the trillian server")
	flag.Parse()

	// setup an empty context
	ctx := context.Background()

	if listenPort > math.MaxUint16 {
		fmt.Fprintf(os.Stderr, "invalid port value '%d'\n", listenPort)
		os.Exit(1)
	}

	// load private key from env variable, should not show up in the history
	privateKeyBase64 = os.Getenv("PRIVATE_KEY")

	// if 'PRIVATE_KEY' is not set, generate a new one
	if len(privateKeyBase64) == 0 {
		fmt.Printf("No 'PRIVATE_KEY' (DER, then base64 encoded) environment variable provided, generating random one in memory\n")
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
		// if 'PRIVATE_KEY' is set, parse the base64 encoded key

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

	// compute SHA256 hash of the insertion key
	// makes the comparison running time independent of the secret value to avoid a side channel attack
	certificateInsertionKeyHash = sha256.Sum256([]byte(certificateInsertionKey))

	// load database url from env variable, should not show up in the history
	databaseUrl = os.Getenv("DATABASE_URL")
	if len(databaseUrl) == 0 {
		fmt.Fprintf(os.Stderr, "no 'DATABASE_URL' environment variable provided\n")
		os.Exit(7)
	}

	// create a db connection pool

	// first parse the config from the URL
	config, err := pgxpool.ParseConfig(databaseUrl)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to parse database url: %v\n", err)
		os.Exit(8)
	}
	// optionally set some values on each new connection
	config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return nil
	}

	// initialize the db pool with the parsed config
	dbPool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to create connection pool: %v\n", err)
		os.Exit(8)
	}

	// ensure the connection is working
	err = dbPool.Ping(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to connect to the database: %v\n", err)
		os.Exit(9)
	}

	// close the database connection when terminating
	defer dbPool.Close()

	// ensure the existence of a consistency tree service
	consistencyClient, err := crypto.NewConsistencyTreeClient(trillianAddress, consistencyLogId, privateKey, server.MAXIMUM_MERGE_DELAY)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to create consistency client: %v\n", err)
		os.Exit(10)
	}

	// retrieve the latest SCH
	sch, err := consistencyClient.LatestSignedConsistencyHead(ctx)
	if err != nil {
		// if an error is thrown, it is likely due to the tree not being initialized so let's try that
		err2 := consistencyClient.InitializeLog(ctx)

		// if that also fails, print both errors
		if err2 != nil {
			fmt.Fprintf(os.Stderr, "unable to obtain latest consistency head: %v\n", err)
			fmt.Fprintf(os.Stderr, "unable to initialize log server: %v\n", err2)
			os.Exit(11)
		}
	}

	// if the log was newly initialized, add the current SMH
	if err != nil || sch.Size == 0 {
		// initialize a new transaction
		tx, err := dbPool.BeginTx(ctx, pgx.TxOptions{
			IsoLevel: pgx.Serializable,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to start transaction: %v\n", err)
			os.Exit(12)
		}

		// create a new SMH based on the current root hash
		smh, err := server.CreateNewSMH(time.Now(), tx, privateKey, context.Background())
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to create new SMH: %v\n", err)
			os.Exit(13)
		}

		// commit the transaction
		err = tx.Commit(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to commit transaction: %v\n", err)
			os.Exit(14)
		}

		// and insert the new SMH into the empty consistency tree
		sch, err = consistencyClient.AppendSignedMapHead(ctx, smh)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to append new SMH: %v\n", err)
			os.Exit(15)
		}
	}

	// cache the current SMH
	smh, err := consistencyClient.LatestSignedMapHead(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to obtain latest signed map head: %v\n", err)
		os.Exit(16)
	}

	// ensure the current SMH matches the current key
	if !smh.Verify(&privateKey.PublicKey) {
		fmt.Fprintf(os.Stderr, "cannot verify the signature on the latest SMH, did the private key change?\n")
		os.Exit(17)
	}

	// cache the inclusion proof for the current SMH
	proof, err := consistencyClient.ProveSignedMapHeadInclusion(context.Background(), sch.Size, smh)
	if err != nil {
		fmt.Fprintf(os.Stderr, "obtaining a proof of inclusion for consistency tree failed: %v\n", err)
		os.Exit(18)
	}

	// marshal the current inclusion proof
	inclusionProof, err := proto.Marshal(proof)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed marshaling inclusion proof: %v\n", err)
		os.Exit(19)
	}

	fmt.Printf("Serving data with SMH:\n%s\n\n", smh.String())

	// all checks have passed, create handler environment for shared data
	env := &server.EndpointHandlerEnv{
		DbPool:                      dbPool,
		PrivateKey:                  privateKey,
		CertificateInsertionKeyHash: certificateInsertionKeyHash[:],

		ConsistencyClient: consistencyClient,

		CurrentSignedMapHead:         smh.Proto(),
		CurrentSignedConsistencyHead: sch.Proto(),
		SchInclusionProof:            inclusionProof,
	}

	server.StartServer(env, listenAddress, listenPort)
}
