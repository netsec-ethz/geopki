package main

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"time"

	"geopki/pkg/bitstring"
	"geopki/pkg/comm"
	"geopki/pkg/crypto"
	"geopki/pkg/geometry"
)

const (
	// the relative grid size used to compute bit strings based on the query volume
	F_GROW = 1
)

func main() {
	// start measuring time for the total running time of the application
	clientStart := time.Now()

	// CLI arguments described by the help texts below
	var address string
	var longitude, latitude float64
	var radius uint64
	var includeCertificates bool

	var publicKeyBase64 string

	flag.StringVar(&address, "address", "", "The HTTP address of the server to send the request to")
	flag.Float64Var(&longitude, "longitude", 190, "The longitude to query for")
	flag.Float64Var(&latitude, "latitude", 100, "The latitude to query for")
	flag.Uint64Var(&radius, "radius", 10, "The radius for the query in meters")
	flag.BoolVar(&includeCertificates, "include-certificates", false, "Whether to include the certificates")
	flag.StringVar(&publicKeyBase64, "public-key", "", "The public key used to verify the signatures.")
	flag.Parse()

	var publicKey *ecdsa.PublicKey

	// set up timing variables
	// shared start time for measuring durations
	var start time.Time

	// time for computing the query bit strings
	var buildingQuery time.Duration

	// time for sending a request and receiving a response
	var request time.Duration

	// time for locally verifying the response
	var verification time.Duration

	// time for locally verifiying the consistency
	var consistency time.Duration

	// time for running the whole application
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

		// type assertion, will crash on mismatch
		publicKey = decodedPublicKey.(*ecdsa.PublicKey)
	}

	// start measuring 'buildingQuery'
	start = time.Now()

	// set altitude to 0
	query, err := comm.NewQuery(longitude, latitude, 0, radius, F_GROW, &geometry.GdalCircleApproximator{})
	if err != nil {
		log.Fatalf("❌ building query: %v\n", err)
	}

	// and then overwrite min and max altitude to cover the full altitude range
	query.MinAltitude = 0
	query.MaxAltitude = int16(bitstring.C_Z)

	// stop 'buildingQuery' measurement
	buildingQuery = time.Since(start)
	// start measuring 'request'
	start = time.Now()

	response, requestSize, responseSize, err := comm.QueryMapServer(
		address,
		query,
		includeCertificates,
	)
	if err != nil {
		log.Fatalf("❌ request failed: %v\n", err)
	}

	// stop 'request' measurement
	request = time.Since(start)
	// start measuring 'verification'
	start = time.Now()

	// ensure the response is complete with respect to the query and
	// that the server included exactly all data it knows in that area by
	// recomputing the root hash
	certificateHashes, err := crypto.VerifyResponse(response, query, publicKey)
	if err != nil {
		log.Fatalf("❌ response verification failed: %v\n", err)
	}

	// stop 'verification' measurement
	verification = time.Since(start)
	// start measuring 'consistency'
	start = time.Now()

	// ensure the received signed map head is included in the consistency tree
	// by requesting a proof of inclusion for the consistency tree
	err = crypto.EnsureConsistency(response, publicKey)
	if err != nil {
		log.Fatalf("❌ consistency verification failed: %v\n", err)
	}

	// stop 'consistency' measurement
	consistency = time.Since(start)
	// stop 'total' measurement
	total = time.Since(clientStart)

	// count the total number of certificate hashes
	certificateHashCount := 0
	for _, n := range response.Nodes {
		certificateHashCount += len(n.GetCertificateHashes())
	}

	fmt.Printf(
		"%f,%f,%d,%d,%d,%d,%d,%d,%d,%d,%f,%f,%f,%f,%f,%t\n",
		longitude,
		latitude,
		radius,
		requestSize,
		// query bit strings
		len(query.XYBitStrings),
		responseSize,
		// nodes in the response
		len(response.Nodes),
		// number of certificate hashes in the response
		certificateHashCount,
		// number of unique certificate hashes in the response
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
		// whether certificates were fetched as well
		includeCertificates,
	)
}
