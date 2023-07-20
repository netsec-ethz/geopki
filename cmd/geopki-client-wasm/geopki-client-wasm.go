package main

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"syscall/js"

	"geopki/pkg/comm"
	"geopki/pkg/crypto"
)

const (
	F_GROW = 1
)

func main() {
	js.Global().Set("getJSONCertificates", getJSONCertificatesWrapper())
	fmt.Println("Exposed window.getJSONCertificates(address, longitude, latitude, altitude, radius, publicKeyBase64?)")

	// make the program run forever and expose the functions all the time
	<-make(chan bool)
}

func getJSONCertificatesWrapper() js.Func {
	jsonFunc := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) < 5 || len(args) > 6 {
			return "Invalid number of arguments passed"
		}

		address := args[0].String()
		longitude := args[1].Float()
		latitude := args[2].Float()
		altitude := args[3].Float()
		radius := args[4].Int()

		publicKeyBase64 := ""
		if len(args) == 6 {
			publicKeyBase64 = args[5].String()
		}

		// since we are doing async stuff including network requests, we have to return
		// a promise, otherwise there will be a deadlock (https://github.com/golang/go/issues/41310)
		handler := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			resolve := args[0]
			reject := args[1]

			go func() {
				certificates, err := getJSONCertificates(
					address,
					longitude,
					latitude,
					altitude,
					uint64(radius),
					publicKeyBase64,
				)

				if err != nil {
					reject.Invoke(js.Global().Get("Error").New(err.Error()))
					return
				}

				jsArray := make([]interface{}, len(certificates))
				for i, certificate := range certificates {
					jsArray[i] = certificate
				}

				resolve.Invoke(jsArray)
			}()

			// The handler of a Promise doesn't return any value
			return nil
		})

		// Create and return the Promise object
		promiseConstructor := js.Global().Get("Promise")
		return promiseConstructor.New(handler)
	})

	return jsonFunc
}

func getJSONCertificates(
	address string,
	longitude, latitude, altitude float64,
	radius uint64,
	publicKeyBase64 string,
) ([]string, error) {

	var publicKey *ecdsa.PublicKey

	if len(publicKeyBase64) == 0 {
		fmt.Println("🚨 No public key passed as an argument, fetching it from the server.")

		plainResponse, err := http.Get(
			fmt.Sprintf("%s/v1/public-key", address),
		)
		if err != nil {
			return nil, fmt.Errorf(
				"❌ failed sending HTTP GET request to %s: %v",
				address,
				err,
			)
		}

		defer plainResponse.Body.Close()

		responseBody, err := io.ReadAll(plainResponse.Body)
		if err != nil {
			return nil, fmt.Errorf(
				"❌ failed reading response: %v",
				err,
			)
		}

		decodedPublicKey, err := x509.ParsePKIXPublicKey(responseBody)
		if err != nil {
			return nil, fmt.Errorf(
				"❌ failed parsing public key: %v",
				err,
			)
		}

		// type assertion
		publicKey = decodedPublicKey.(*ecdsa.PublicKey)
	} else {
		derPublicKey, err := base64.StdEncoding.DecodeString(publicKeyBase64)
		if err != nil {
			return nil, fmt.Errorf(
				"❌ failed parsing base64 of public key flag: %v",
				err,
			)
		}

		decodedPublicKey, err := x509.ParsePKIXPublicKey(derPublicKey)

		if err != nil {
			return nil, fmt.Errorf(
				"❌ failed parsing public key: %v",
				err,
			)
		}

		// type assertion
		publicKey = decodedPublicKey.(*ecdsa.PublicKey)
	}

	query, err := comm.NewQuery(longitude, latitude, altitude, radius, F_GROW, &comm.S2CircleApproximator{})
	if err != nil {
		return nil, fmt.Errorf("❌ building query: %v\n", err)
	}

	response, requestSize, responseSize, err := comm.QueryMapServerSlow(
		address,
		query,
		true,
	)
	if err != nil {
		return nil, fmt.Errorf("❌ request failed: %v\n", err)
	}

	// ensure the response is complete with respect to the query and
	// that the server included exactly all data it knows in that area by
	// recomputing the root hash
	certificateHashes, err := crypto.VerifyResponse(response, query, publicKey)
	if err != nil {
		return nil, fmt.Errorf("❌ response verification failed: %v\n", err)
	}

	// ensure the received signed map head is included in the consistency tree
	// by requesting a proof of inclusion for the consistency tree
	err = crypto.EnsureConsistency(response, publicKey)
	if err != nil {
		return nil, fmt.Errorf("❌ consistency verification failed: %v\n", err)
	}

	fmt.Printf("✅ Cryptographic verification of response succeeded!\n")

	fmt.Printf("🏋️ Sizes\n")
	fmt.Printf("    Request size: %dB, %d bit strings\n", requestSize, len(query.XYBitStrings))
	fmt.Printf("    Response size: %dB, %d nodes\n", responseSize, len(response.Nodes))
	fmt.Printf("    Consistency proof response size: %dB\n", len(response.InclusionProof))

	fmt.Printf("📡 Received %d certificate hashes:\n", certificateHashes.Cardinality())
	for certificateHash := range certificateHashes.Iter() {
		fmt.Printf("    %s\n", certificateHash)
	}

	fmt.Printf("📡 Received %d certificates\n", len(response.GetCertificates()))

	results := make([]string, len(response.GetCertificates()))

	for i, rawCertificate := range response.GetCertificates() {
		// TODO: later this will probably parse a x509 certificate
		certificate, err := crypto.UnmarshalGeoCertificate(rawCertificate)
		if err != nil {
			return nil, fmt.Errorf("❌ failed parsing certificate: %v\n", err)
		}

		results[i] = certificate.JSON()
	}

	return results, nil
}
