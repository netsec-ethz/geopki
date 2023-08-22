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
	// the relative grid size used to compute bit strings based on the query volume
	F_GROW = 1
)

func main() {
	// expose 'getJSONCertificates' on the window object
	js.Global().Set("getJSONCertificates", getJSONCertificatesWrapper())
	fmt.Println("Exposed window.getJSONCertificates(address, longitude, latitude, altitude, radius, publicKeyBase64?)")

	// make the program run forever to expose the function during the whole time the window is open
	<-make(chan bool)
}

// returns a javascript function returning a promise that resolves to an array of geo certs
func getJSONCertificatesWrapper() js.Func {

	// define the javascript function
	jsFunc := js.FuncOf(func(this js.Value, args []js.Value) any {

		// validate the number of input arguments
		if len(args) < 5 || len(args) > 6 {
			return "Invalid number of arguments passed"
		}

		// validate the type of the arguments
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
			// the function to sucessfully resolve the promise
			resolve := args[0]

			// the function to reject the promise in case of an error
			reject := args[1]

			// perform query asynchronously
			go func() {
				// query server for geo certificates
				certificates, err := getJSONCertificates(
					address,
					longitude,
					latitude,
					altitude,
					uint64(radius),
					publicKeyBase64,
				)
				if err != nil {
					// reject the promise in case of an error
					reject.Invoke(js.Global().Get("Error").New(err.Error()))
					return
				}

				// convert the received data to a javascript array
				jsArray := make([]interface{}, len(certificates))
				for i, certificate := range certificates {
					jsArray[i] = certificate
				}

				resolve.Invoke(jsArray)
			}()

			// the handler of a Promise doesn't return any value
			return nil
		})

		// create and return the promise object
		promiseConstructor := js.Global().Get("Promise")
		return promiseConstructor.New(handler)
	})

	return jsFunc
}

// fetches geo certificates from the map server
func getJSONCertificates(
	// the map server's address
	address string,
	// the coordinates of the query center
	longitude, latitude, altitude float64,
	// the query radius
	radius uint64,
	// the base64 encoded public key or an empty string
	publicKeyBase64 string,
) ([]string, error) {

	// the map server's public key
	var publicKey *ecdsa.PublicKey

	// if the empty string was passed, fetch the public key from the map server
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

		// type assertion, crashes if the type does not match
		publicKey = decodedPublicKey.(*ecdsa.PublicKey)
	} else {
		// if a public key was passed as a CLI argument, decode it
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

		// type assertion, crashes if the type does not match
		publicKey = decodedPublicKey.(*ecdsa.PublicKey)
	}

	// compute the query based on the input coordinates, radius and the relative grid size
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

	// iterate over received certificates and return them JSON formatted
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
