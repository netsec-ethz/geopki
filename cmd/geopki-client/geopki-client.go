package main

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"geopki/pkg/comm"
	"geopki/pkg/crypto"
	"io"
	"log"
	"math"
	"net/http"
	"time"
)

const (
	F_GROW = 1
)

func main() {
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

	// set up timing variables
	var fetchingDecodingPublicKeyStart time.Time
	var fetchingDecodingPublicKey time.Duration

	var buildingQueryStart time.Time
	var buildingQuery time.Duration

	var requestStart time.Time
	var request time.Duration

	var verificationStart time.Time
	var verification time.Duration

	fetchingDecodingPublicKeyStart = time.Now()

	if len(publicKeyBase64) == 0 {
		fmt.Println("🚨 No public key passed as an argument, fetching it from the server.")

		plainResponse, err := http.Get(
			fmt.Sprintf("%s/v1/public-key", address),
		)
		if err != nil {
			log.Fatalf(
				"❌ failed sending HTTP GET request to %s: %v",
				address,
				err,
			)
		}

		defer plainResponse.Body.Close()

		responseBody, err := io.ReadAll(plainResponse.Body)
		if err != nil {
			log.Fatalf(
				"❌ failed reading response: %v",
				err,
			)
		}

		decodedPublicKey, err := x509.ParsePKIXPublicKey(responseBody)
		if err != nil {
			log.Fatalf(
				"❌ failed parsing public key: %v",
				err,
			)
		}

		// type assertion
		publicKey = decodedPublicKey.(*ecdsa.PublicKey)
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

	fetchingDecodingPublicKey = time.Since(fetchingDecodingPublicKeyStart)
	buildingQueryStart = time.Now()

	query, err := comm.NewQuery(longitude, latitude, altitude, radius, F_GROW)
	if err != nil {
		log.Fatalf("❌ building fquery: %v\n", err)
	}

	buildingQuery = time.Since(buildingQueryStart)
	requestStart = time.Now()

	response, err := comm.QueryMapServer(
		address,
		query,
		includeCertificates,
	)
	if err != nil {
		log.Fatalf("❌ request failed: %v\n", err)
	}

	request = time.Since(requestStart)
	verificationStart = time.Now()

	certificateHashes, err := crypto.VerifyResponse(response, query, publicKey)
	if err != nil {
		log.Fatalf("❌ response verification failed: %v\n", err)
	}

	verification = time.Since(verificationStart)

	fmt.Printf("✅ Cryptographic verification of response succeeded!\n")

	fmt.Printf("📡 Received %d certificate hashes:\n", certificateHashes.Cardinality())
	for _, certificateHash := range certificateHashes.ToSlice() {
		fmt.Printf("    %s\n", certificateHash)
	}

	fmt.Printf("📡 Received %d certificates\n", len(response.GetCertificates()))
	for _, rawCertificate := range response.GetCertificates() {
		// TODO: later this will probably parse a x509 certificate
		certificate := new(crypto.GeoCertificate)
		json.Unmarshal(rawCertificate, certificate)

		fmt.Printf("    %s, %s\n", certificate.Domain, certificate.Certificate_id)
	}

	fmt.Printf("⌛️ Timing\n")
	fmt.Printf("    Fetch & Parse Public Key: %fs\n", fetchingDecodingPublicKey.Seconds())
	fmt.Printf("    Build Query: %fs\n", buildingQuery.Seconds())
	fmt.Printf("    Send Request & Receive Response: %fs\n", request.Seconds())
	fmt.Printf("    Verify Response: %fs\n", verification.Seconds())
}
