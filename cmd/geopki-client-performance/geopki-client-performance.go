package main

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"math"
	"time"

	"geopki/pkg/comm"
	"geopki/pkg/crypto"
	"geopki/pkg/geometry"
)

const (
	F_GROW = 0.1
)

func main() {
	clientStart := time.Now()

	var address string
	var longitude, latitude, altitude float64
	var radius uint64
	var includeCertificates bool

	var publicKeyBase64 string

	flag.StringVar(&address, "address", "", "The HTTP address of the server to send the request to")
	flag.Float64Var(&longitude, "longitude", 190, "The longitude to query for")
	flag.Float64Var(&latitude, "latitude", 100, "The latitude to query for")
	flag.Float64Var(&altitude, "altitude", math.Inf(0), "The altitude to query for")
	flag.Uint64Var(&radius, "radius", 10, "The radius for the query in meters")
	flag.BoolVar(&includeCertificates, "include-certificates", false, "Whether to include the certificates")
	flag.StringVar(&publicKeyBase64, "public-key", "", "The public key used to verify the signatures.")
	flag.Parse()

	var publicKey *ecdsa.PublicKey

	// set up additional timing variables

	// used as the start time for all measurements except the one for the whole program
	var start time.Time

	var buildingQuery time.Duration
	var request time.Duration
	var verification time.Duration
	var consistency time.Duration
	var total time.Duration

	if len(publicKeyBase64) == 0 {
		log.Fatalf("❌ no public key was passed")
	} else {
		derPublicKey, err := base64.StdEncoding.DecodeString(publicKeyBase64)
		if err != nil {
			log.Fatalf(
				"❌ failed parsing base64 of public key flag: %v",
				err,
			)
		}

		decodedPublicKey, err := x509.ParsePKIXPublicKey(derPublicKey)

		if err != nil {
			log.Fatalf(
				"❌ failed parsing public key: %v",
				err,
			)
		}

		// type assertion
		publicKey = decodedPublicKey.(*ecdsa.PublicKey)
	}
	start = time.Now()

	query, err := comm.NewQuery(longitude, latitude, altitude, radius, F_GROW, &geometry.GdalCircleApproximator{})
	if err != nil {
		log.Fatalf("❌ building query: %v\n", err)
	}

	buildingQuery = time.Since(start)
	start = time.Now()

	response, requestSize, responseSize, err := comm.QueryMapServer(
		address,
		query,
		includeCertificates,
	)
	if err != nil {
		log.Fatalf("❌ request failed: %v\n", err)
	}

	request = time.Since(start)
	start = time.Now()

	// ensure the response is complete with respect to the query and
	// that the server included exactly all data it knows in that area by
	// recomputing the root hash
	certificateHashes, err := crypto.VerifyResponse(response, query, publicKey)
	if err != nil {
		log.Fatalf("❌ response verification failed: %v\n", err)
	}

	verification = time.Since(start)
	start = time.Now()

	// ensure the received signed map head is included in the consistency tree
	// by requesting a proof of inclusion for the consistency tree
	err = crypto.EnsureConsistency(response, publicKey)
	if err != nil {
		log.Fatalf("❌ consistency verification failed: %v\n", err)
	}

	consistency = time.Since(start)
	total = time.Since(clientStart)

	fmt.Printf(
		"%f,%f,%f,%d,%d,%d,%d,%d,%d,%d,%f,%f,%f,%f,%f\n",
		longitude,
		latitude,
		altitude,
		radius,
		requestSize,
		// query bit strings
		len(query.XYBitStrings),
		responseSize,
		// nodes in the response
		len(response.Nodes),
		// number of certificate hashes in the response
		certificateHashes.Cardinality(),
		// size of the consistency proof
		len(response.InclusionProof),
		// time to build the query
		buildingQuery.Seconds(),
		// time to send & receive the request
		request.Seconds(),
		// time to verify the response's correctness
		verification.Seconds(),
		// time to verify the consistency proof
		consistency.Seconds(),
		// total client running time
		total.Seconds(),
	)
}
