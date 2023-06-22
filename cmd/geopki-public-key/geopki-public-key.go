package main

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
)

const (
	F_GROW = 0.1
)

func main() {
	var address string

	flag.StringVar(&address, "address", "", "The HTTP address of the server to send the request to")
	flag.Parse()

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

	// type assertion, check if key i valid
	_, ok := decodedPublicKey.(*ecdsa.PublicKey)
	if !ok {
		log.Fatalf("❌ received invalid key type: %+v", decodedPublicKey)
	}

	fmt.Printf("public key of %s:\n%s\n", address, base64.StdEncoding.EncodeToString(responseBody))
}
