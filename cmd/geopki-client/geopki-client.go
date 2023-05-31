package main

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"flag"
	"fmt"
	"geopki/pkg/comm"
	"geopki/pkg/crypto"
	"io"
	"log"
	"math"
	"net/http"

	mapset "github.com/deckarep/golang-set/v2"
)

const (
	F_GROW = 1
)

func main() {
	var address string
	var longitude, latitude, altitude float64
	var radius uint64

	var publicKeyBase64 string

	flag.StringVar(&address, "address", "", "The HTTP address of the server to send the request to")
	flag.Float64Var(&longitude, "longitude", 190, "The longitude to query for")
	flag.Float64Var(&latitude, "latitude", 100, "The latitude to query for")
	flag.Float64Var(&altitude, "altitude", math.Inf(0), "The altitude to query for")
	flag.Uint64Var(&radius, "radius", 10, "The radius for the query in meters")
	flag.StringVar(&publicKeyBase64, "public-key", "", "The public key used to verify the signatures.")
	flag.Parse()

	var publicKey *ecdsa.PublicKey

	if len(publicKeyBase64) == 0 {
		fmt.Println("🚨 No public key passed as an argument, fetching it from the server.")

		plainResponse, err := http.Get(
			fmt.Sprintf("%s/v1/public-key", address),
		)
		if err != nil {
			log.Fatalf(
				"failed sending HTTP GET request to %s: %v",
				address,
				err,
			)
		}

		defer plainResponse.Body.Close()

		responseBody, err := io.ReadAll(plainResponse.Body)
		if err != nil {
			log.Fatalf(
				"failed reading response: %v",
				err,
			)
		}

		decodedPublicKey, err := x509.ParsePKIXPublicKey(responseBody)
		if err != nil {
			log.Fatalf(
				"failed parsing public key: %v",
				err,
			)
		}

		// type assertion
		publicKey = decodedPublicKey.(*ecdsa.PublicKey)
	} else {
		derPublicKey, err := base64.StdEncoding.DecodeString(publicKeyBase64)
		if err != nil {
			log.Fatalf(
				"failed parsing base64 of public key flag: %v",
				err,
			)
		}

		decodedPublicKey, err := x509.ParsePKIXPublicKey(derPublicKey)

		if err != nil {
			log.Fatalf(
				"failed parsing public key: %v",
				err,
			)
		}

		// type assertion
		publicKey = decodedPublicKey.(*ecdsa.PublicKey)
	}

	response, err := comm.Query(
		address,
		longitude,
		latitude,
		altitude,
		radius,
		F_GROW,
	)
	if err != nil {
		log.Fatalf("request failed: %v\n", err)
	}

	err = crypto.VerifyResponse(response, publicKey)
	if err != nil {
		log.Fatalf("response verification failed: %v\n", err)
	}

	fmt.Printf("✅ Cryptographic verification of response succeeded!\n")

	// filter out duplicate hashes, a single certificate can be stored
	// at multiple nodes
	certificateHashes := mapset.NewSet[string]()
	for _, node := range response.Nodes {
		for _, certificateHash := range node.CertificateHashes {
			certificateHashes.Add(hex.EncodeToString(certificateHash))
		}
	}

	fmt.Printf("🚀 Received %d certificate hashes:\n", certificateHashes.Cardinality())
	for _, certificateHash := range certificateHashes.ToSlice() {
		fmt.Printf("    %s\n", certificateHash)
	}
	fmt.Printf("🚀 Received %d certificates\n", len(response.Certificates))
}
