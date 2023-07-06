package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	var address string
	var insertionKey string

	flag.StringVar(&address, "address", "http://localhost:1234", "The address of the server where the certificates should be imported")
	flag.StringVar(&insertionKey, "insertion-key", "", "The insetion key")
	flag.Parse()

	if insertionKey == "" {
		log.Fatalf("insertion key must be set")
	}

	fmt.Printf("Build indices, compute hashes and remove expired certificates.\n")
	start := time.Now()
	fmt.Printf("This can take quite some time..\n")

	plainResponse, err := http.Post(
		fmt.Sprintf("%s/v1/release?key=%s", address, insertionKey),
		"application/json",
		nil,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed sending HTTP GET request to %s: %v\n", address, err)
		os.Exit(1)
	}

	body, err := io.ReadAll(plainResponse.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading response body failed: %v\n", err)
		os.Exit(1)
	}

	if plainResponse.StatusCode != 200 {
		fmt.Fprintf(os.Stderr, "server replied with %s\n", string(body))
		os.Exit(1)
	}

	fmt.Printf("Done, built indices and computed hashes in %f minutes.\n", time.Since(start).Minutes())
}
