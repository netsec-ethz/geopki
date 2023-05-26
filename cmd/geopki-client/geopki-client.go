package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"

	"geopki/pkg/bitstring"
	"geopki/pkg/comm"

	"github.com/golang/geo/s2"
	"google.golang.org/protobuf/proto"
)

type ErrorResponse struct {
	Error string
}

const (
	F_GROW = 1
)

func main() {
	var address string
	var longitude, latitude, altitude float64
	var radius uint64

	flag.StringVar(&address, "address", "", "The HTTP adress of the server to send the request to")
	flag.Float64Var(&longitude, "longitude", 190, "The longitude to query for")
	flag.Float64Var(&latitude, "latitude", 100, "The latitude to query for")
	flag.Float64Var(&altitude, "altitude", math.Inf(0), "The altitude to query for")
	flag.Uint64Var(&radius, "radius", 10, "The radius for the query in meters")
	flag.Parse()

	if address == "" {
		log.Fatal("Missing address value, use --address=http://...")
	}

	if longitude < -180 || longitude > 180 {
		log.Fatal("Invalid longitude value, must be in the range [-180, 180]")
	}

	if latitude < -90 || latitude > 90 {
		log.Fatal("Invalid latitude value, must be in the range [-90, 90]")
	}

	if altitude < float64(bitstring.D) || altitude > float64(bitstring.H) {
		log.Fatalf(
			"Invalid altitude value, must be in the range [%d, %d]",
			bitstring.D,
			bitstring.H,
		)
	}

	if radius > math.MaxUint8 {
		log.Fatal("Invalid radius value, must be smaller than 255")
	}

	sphere := bitstring.ApproximateSphere(
		longitude,
		latitude,
		uint8(radius),
		16,
	)

	bitStrings, err := bitstring.PolygonsTo2DBitStrings(
		[]*s2.Loop{sphere},
		F_GROW,
	)
	if err != nil {
		log.Fatalf(
			"Failed approximating the sphere: %v\n",
			err,
		)
	}

	altitudeInt := int16(altitude)

	minAltitude := altitudeInt - bitstring.D - int16(radius)
	maxAltitude := altitudeInt - bitstring.D + int16(radius)

	queries := make([]*comm.XYBitString, len(bitStrings))
	for i, bitString := range bitStrings {
		queries[i] = &comm.XYBitString{
			XYBitString:    bitString.XYBitString,
			XYBitStringLen: uint32(bitString.XYBitStringLen),
		}
	}

	request, err := proto.Marshal(&comm.Request{
		XYBitStrings: queries,
		MinAltitude:  uint32(minAltitude),
		MaxAltitude:  uint32(maxAltitude),
	})

	if err != nil {
		log.Fatalf(
			"Failed marshalling message: %v\n",
			err,
		)
	}

	log.Fatalf("success, computed %d bit strings", len(bitStrings))

	plainResponse, err := http.Post(
		"http://localhost:1234/v1/get-bit-strings",
		"application/octet-stream",
		bytes.NewBuffer(request),
	)
	if err != nil {
		log.Fatalf(
			"Failed sending HTTP POST request to %s: %v\n",
			address,
			err,
		)
	}

	defer plainResponse.Body.Close()

	responseBody, err := io.ReadAll(plainResponse.Body)
	if err != nil {
		log.Fatalf(
			"Failed reading response: %v\n",
			err,
		)
	}

	response := new(comm.Response)
	err = proto.Unmarshal(responseBody, response)

	if err != nil {
		var errorResponse ErrorResponse
		err = json.Unmarshal(responseBody, &errorResponse)
		if err != nil {
			log.Fatalf(
				"Failed unmarshalling: %v\n",
				err,
			)
		}

		log.Fatalf("Received error message: %s\n", errorResponse.Error)
	}

	print("received response:")
	fmt.Printf("%x", responseBody)
}
