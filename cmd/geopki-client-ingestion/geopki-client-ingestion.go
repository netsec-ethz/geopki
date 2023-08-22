package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"geopki/pkg/crypto"
	"net/http"
	"os"
	"time"
)

// checks if the certificates are properly json encoded
// returns the number of certificates
func checkValidity(certificatesJson []byte) (int, error) {
	var certificates []*crypto.GeoCertificate
	err := json.Unmarshal(certificatesJson, &certificates)

	return len(certificates), err
}

func main() {

	var address string
	var insertionKey string
	var certificatesInput string
	var certificatesJson []byte

	flag.StringVar(&address, "address", "", "The HTTP address of the server to send the request to")
	flag.StringVar(&insertionKey, "insertion-key", "", "The insertion key")
	flag.StringVar(&certificatesInput, "certificates", "[]", "The certificates that should be inserted")
	flag.Parse()

	// check if 'certificatesInput' points to a file
	if _, err := os.Stat(certificatesInput); err == nil {
		// if it does, read json from disk
		content, err := os.ReadFile(certificatesInput)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed reading file '%s': %v\n", certificatesInput, err)
			os.Exit(1)
		}

		certificatesJson = content
	} else {
		// if it doesn't interpret the input as json directly
		certificatesJson = []byte(certificatesInput)
	}

	// verify the input is valid json
	certificateCount, err := checkValidity(certificatesJson)
	if err != nil {
		fmt.Fprintf(os.Stderr, "received invalid certificate set '%s': %v\n", certificatesInput, err)
		os.Exit(1)
	}

	// gzip 'certificatesJson' before sending it to the server
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed creating gzip writer: %v\n", err)
		os.Exit(1)
	}

	_, err = zw.Write(certificatesJson)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed compressing certificates: %v\n", err)
		os.Exit(1)
	}

	if err := zw.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "failed closing gzip writer: %v\n", err)
		os.Exit(1)
	}

	// start measuring ingestion time
	start := time.Now()

	// send request to ingest data
	plainResponse, err := http.Post(
		fmt.Sprintf("%s/v1/insert?key=%s", address, insertionKey),
		"application/json",
		&buf,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed sending HTTP POST request to %s: %v", address, err)
		os.Exit(1)
	}

	// stop measuring ingestion time
	runningTime := time.Since(start)

	fmt.Printf(
		"%d,%f,%t\n",
		certificateCount,
		runningTime.Seconds(),
		(plainResponse.StatusCode == 200),
	)
}
