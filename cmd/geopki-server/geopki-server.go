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

	server "geopki/internal/geopki-server"
	"geopki/pkg/crypto"
	"geopki/pkg/database"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"
)

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
	config, err := pgxpool.ParseConfig(databaseUrl)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to parse database url: %v\n", err)
		os.Exit(8)
	}
	config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return nil
	}

	dbPool, err := pgxpool.NewWithConfig(context.Background(), config)
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

	dirty, err := database.QueryState(server.DATABASE_STATE_KEY_DIRTY, tx, ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to query persistent database state '%s': %v\n", server.DATABASE_STATE_KEY_DIRTY, err)
		os.Exit(11)
	}

	err = tx.Commit(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to commit transaction: %v\n", err)
		os.Exit(12)
	}

	// ensure the existence of a consistency tree service
	consistencyClient, err := crypto.NewConsistencyTreeClient(trillianAddress, consistencyLogId, privateKey, server.MAXIMUM_MERGE_DELAY)
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

		smh, err := server.CreateNewSMH(tx, privateKey, context.Background())
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to create new SMH: %v\n", err)
			os.Exit(16)
		}

		err = tx.Commit(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to commit transaction: %v\n", err)
			os.Exit(17)
		}

		// insert it into the consistency tree
		sch, err = consistencyClient.AppendSignedMapHead(ctx, smh)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to append new SMH: %v\n", err)
			os.Exit(18)
		}
	}

	// cache the current smh
	smh, err := consistencyClient.LatestSignedMapHead(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to obtain latest signed map head: %v\n", err)
		os.Exit(19)
	}

	if !smh.Verify(&privateKey.PublicKey) {
		fmt.Fprintf(os.Stderr, "cannot verify the signature on the latest SMH, did the private key change?\n")
		os.Exit(20)
	}

	proof, err := consistencyClient.ProveSignedMapHeadInclusion(context.Background(), sch.Size, smh)
	if err != nil {
		fmt.Fprintf(os.Stderr, "obtaining a proof of inclusion for consistency tree failed: %v\n", err)
		os.Exit(21)
	}

	inclusionProof, err := proto.Marshal(proof)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed marshaling inclusion proof: %v\n", err)
		os.Exit(22)
	}

	fmt.Printf("Serving data with SMH:\n%s\n\n", smh.String())

	// create handler environment for shared data
	env := &server.EndpointHandlerEnv{
		DbPool:                      dbPool,
		PrivateKey:                  privateKey,
		CertificateInsertionKeyHash: certificateInsertionKeyHash[:],

		ConsistencyClient: consistencyClient,

		CurrentSignedMapHead:         smh,
		CurrentSignedConsistencyHead: sch,
		SchInclusionProof:            inclusionProof,

		IsDirty: dirty == "true",
	}

	server.StartServer(env, listenAddress, listenPort)
}
