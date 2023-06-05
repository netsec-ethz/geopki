package comm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"geopki/pkg/bitstring"
	"io"
	"math"
	"net/http"

	"github.com/golang/geo/s2"
	"google.golang.org/protobuf/proto"
)

type ErrorResponse struct {
	Error string
}

type Query struct {
	XYBitStrings             []bitstring.RawXYBitString
	MinAltitude, MaxAltitude int16
}

func NewQuery(
	longitude, latitude, altitude float64,
	radius uint64,
	fGrow float64,
) (*Query, error) {
	if longitude < -180 || longitude > 180 {
		return nil, fmt.Errorf("invalid longitude value, must be in the range [-180, 180]")
	}

	if latitude < -90 || latitude > 90 {
		return nil, fmt.Errorf("invalid latitude value, must be in the range [-90, 90]")
	}

	if altitude < float64(bitstring.D) || altitude > float64(bitstring.H) {
		return nil, fmt.Errorf(
			"invalid altitude value, must be in the range [%d, %d]",
			bitstring.D,
			bitstring.H,
		)
	}

	if radius > math.MaxUint8 {
		return nil, fmt.Errorf("invalid radius value, must be smaller than 255")
	}

	sphere := bitstring.ApproximateSphere(
		longitude,
		latitude,
		uint8(radius),
		16,
	)

	bitStrings, err := bitstring.PolygonsTo2DBitStrings(
		[]*s2.Loop{sphere},
		fGrow,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed approximating the sphere: %v",
			err,
		)
	}

	altitudeInt := int16(altitude)

	query := &Query{
		XYBitStrings: bitStrings,
		MinAltitude:  altitudeInt - bitstring.D - int16(radius),
		MaxAltitude:  altitudeInt - bitstring.D + int16(radius),
	}

	return query, nil
}

func QueryMapServer(
	address string,
	query *Query,
	includeCertificates bool,
) (*Response, int, int, error) {
	if address == "" {
		return nil, 0, 0, fmt.Errorf("missing address value, use --address=http://[...]")
	}

	queries := make([]*XYBitString, len(query.XYBitStrings))
	for i, bitString := range query.XYBitStrings {
		queries[i] = &XYBitString{
			XYBitString:    bitString.XYBitString,
			XYBitStringLen: uint32(bitString.XYBitStringLen),
		}
	}

	request, err := proto.Marshal(&Request{
		XYBitStrings: queries,
		MinAltitude:  uint32(query.MinAltitude),
		MaxAltitude:  uint32(query.MaxAltitude),
	})

	if err != nil {
		return nil, 0, 0, fmt.Errorf(
			"failed marshalling message: %v",
			err,
		)
	}

	// log.Fatalf("success, computed %d bit strings", len(bitStrings))

	getParameters := ""
	if includeCertificates {
		getParameters = "?include-certificates"
	}

	plainResponse, err := http.Post(
		fmt.Sprintf("%s/v1/query%s", address, getParameters),
		"application/octet-stream",
		bytes.NewBuffer(request),
	)
	if err != nil {
		return nil, 0, 0, fmt.Errorf(
			"failed sending HTTP POST request to %s: %v",
			address,
			err,
		)
	}

	defer plainResponse.Body.Close()

	responseBody, err := io.ReadAll(plainResponse.Body)
	if err != nil {
		return nil, 0, 0, fmt.Errorf(
			"failed reading response: %v",
			err,
		)
	}

	response := new(Response)
	err = proto.Unmarshal(responseBody, response)

	if err != nil {
		var errorResponse ErrorResponse
		err = json.Unmarshal(responseBody, &errorResponse)
		if err != nil {
			return nil, 0, 0, fmt.Errorf(
				"failed unmarshalling: %v",
				err,
			)
		}

		return nil, 0, 0, fmt.Errorf("received error message: %s", errorResponse.Error)
	}

	return response, len(request), len(responseBody), nil
}

func ParseQuery(
	query []byte,
) ([]*XYBitString, uint16, uint16, error) {
	request := new(Request)
	err := proto.Unmarshal(query, request)
	if err != nil {
		return nil, 0, 0, err
	}

	if request.MaxAltitude > uint32(bitstring.C_Z) || request.MinAltitude > uint32(bitstring.C_Z) {
		return nil, 0, 0, fmt.Errorf("altitude values cannot be greater than %d", bitstring.C_Z)
	}

	requestBitStringPairs := request.GetXYBitStrings()

	if len(requestBitStringPairs) == 0 {
		return nil, 0, 0, fmt.Errorf("requested no xy bit string pairs")
	}

	for i, requestBitStringPairs := range requestBitStringPairs {
		if requestBitStringPairs.XYBitStringLen > 51 {
			return nil, 0, 0, fmt.Errorf("bit string length of at index %d is %d which is > 51", i, requestBitStringPairs.XYBitStringLen)
		}
	}

	var minAltitude uint16 = uint16(request.MinAltitude)
	var maxAltitude uint16 = uint16(request.MaxAltitude)

	return requestBitStringPairs, minAltitude, maxAltitude, nil
}
