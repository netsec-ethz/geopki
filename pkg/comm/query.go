package comm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"geopki/pkg/bitstring"
	"io"
	"math"
	"net/http"

	"github.com/valyala/fasthttp"
	"google.golang.org/protobuf/proto"
)

const (
	// factor for accounting for radius inaccuracies arising from the surface projection
	// from below ground
	RADIUS_ERROR_FACTOR = 1.005
)

type ErrorResponse struct {
	Error string
}

type Query struct {
	XYBitStrings             []bitstring.RawXYBitString
	MinAltitude, MaxAltitude int16
}

// generalized approximator interface which allows any 2d geometry to be returned
// allows the wasm client to use S2 and the rest GDAL for computing polygon intersections
type CircleApproximator interface {
	ApproximateCircle(longitude, latitude float64, radiusM uint8) (bitstring.Geometry2D, error)
}

type S2CircleApproximator struct{}

func (s *S2CircleApproximator) ApproximateCircle(longitude, latitude float64, radiusM uint8) (bitstring.Geometry2D, error) {
	sphere := bitstring.ApproximateCircle(
		longitude,
		latitude,
		radiusM,
		// use 64-sided polygon
		16,
	)

	return &bitstring.S2Geometry2D{
		Loop: sphere,
	}, nil
}

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func NewQuery(
	longitude, latitude, altitude float64,
	radius uint64,
	fGrow float64,
	// the circle estimator returns some 2d polygon approximating the circle
	// dependency injection allows returning either a S2 or GDAL geometry which
	// impacts the accuracy of the polygon intersection computations
	circleApproximator CircleApproximator,
) (*Query, error) {
	if longitude < -180 || longitude > 180 {
		return nil, fmt.Errorf("invalid longitude value, must be in the range [-180, 180]")
	}

	if latitude < -90 || latitude > 90 {
		return nil, fmt.Errorf("invalid latitude value, must be in the range [-90, 90]")
	}

	if altitude < float64(bitstring.D) || altitude > float64(bitstring.H) {
		return nil, fmt.Errorf(
			"invalid altitude value, must be in the range [%d, %d], %f given",
			bitstring.D,
			bitstring.H,
			altitude,
		)
	}

	radius = uint64(math.Ceil(float64(radius) * RADIUS_ERROR_FACTOR))

	if radius > math.MaxUint8 {
		return nil, fmt.Errorf("invalid radius value, must be smaller than %d", int(math.Floor(float64(255/RADIUS_ERROR_FACTOR))))
	}

	sphere, err := circleApproximator.ApproximateCircle(longitude, latitude, uint8(radius))
	if err != nil {
		return nil, fmt.Errorf("failed approximating circle: %v", err)
	}

	// free sphere memory, no longer needed
	defer sphere.Destroy()

	bitStrings, err := bitstring.PolygonsTo2DBitStrings(
		[]bitstring.Geometry2D{sphere},
		fGrow,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed approximating the sphere: %v",
			err,
		)
	}

	minAltitude := int64(altitude) - int64(bitstring.D) - int64(radius)
	maxAltitude := int64(altitude) - int64(bitstring.D) + int64(radius)

	query := &Query{
		XYBitStrings: bitStrings,
		MinAltitude:  int16(max(minAltitude, 0)),
		MaxAltitude:  int16(min(maxAltitude, int64(bitstring.C_Z))),
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
			"failed marshaling message: %v",
			err,
		)
	}

	getParameters := ""
	if includeCertificates {
		getParameters = "?c"
	}

	httpRquest := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(httpRquest)

	httpRquest.Header.SetMethod("POST")
	httpRquest.SetBody(request)
	httpRquest.SetRequestURI(fmt.Sprintf("%s/v1/query%s", address, getParameters))

	httpResponse := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(httpResponse)

	err = fasthttp.Do(httpRquest, httpResponse)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("request failed: %s", err.Error())
	}

	responseBody := httpResponse.Body()
	response := new(Response)
	err = proto.Unmarshal(responseBody, response)

	if err != nil {
		var errorResponse ErrorResponse
		err = json.Unmarshal(responseBody, &errorResponse)
		if err != nil {
			return nil, 0, 0, fmt.Errorf(
				"failed unmarshaling: %v",
				err,
			)
		}

		return nil, 0, 0, fmt.Errorf("received error message: %s", errorResponse.Error)
	}

	return response, len(request), len(responseBody), nil
}

func QueryMapServerSlow(
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
			"failed marshaling message: %v",
			err,
		)
	}

	getParameters := ""
	if includeCertificates {
		getParameters = "?c"
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
				"failed unmarshaling: %v",
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
