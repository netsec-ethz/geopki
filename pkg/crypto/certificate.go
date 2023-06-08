package crypto

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"geopki/pkg/bitstring"

	"github.com/golang/geo/s2"
)

// a linear ring as defined in https://www.rfc-editor.org/rfc/rfc7946#section-3.1.6
type LinearRing = [][2]float64

// the first element is a list of exterior rings, the second one a list of interior rings
type ExteriorInteriorRings = [2][]LinearRing

type GeoCertArea struct {
	Coordinates ExteriorInteriorRings `json:"coordinates"`
	Type        string                `json:"type"`
}

func (area *GeoCertArea) Loops() ([]*s2.Loop, error) {
	if area.Type != "MultiPolygon" {
		return nil, fmt.Errorf("area.Type must be equal to 'MultiPolygon'")
	}

	exteriorRing := area.Coordinates[0]

	loops := make([]*s2.Loop, len(exteriorRing))
	for i, polygon := range exteriorRing {

		// remove last point that is equal to the first and not needed by s2
		polygon = polygon[:len(polygon)-1]

		pts := make([]s2.Point, len(polygon))
		for i, pt := range polygon {
			// geojson is (longitude, latitude), see https://datatracker.ietf.org/doc/html/rfc7946#section-3.1.1
			pts[i] = s2.PointFromLatLng(s2.LatLngFromDegrees(pt[1], pt[0]))
		}

		loops[i] = s2.LoopFromPoints(pts)
	}

	return loops, nil
}

// this is a mock certificate format until proper x509 extensions are defined
type GeoCertificate struct {
	Certificate_id string `json:"certificate_id"`
	Domain         string `json:"domain"`

	// a list of areas associated with this certificate
	Areas []GeoCertArea `json:"areas"`
	// for each of the areas the minimum and maximum altitudes
	AreasAltitude [][2]float64 `json:"areas_altitude"`

	NotValidAfter string `json:"not_valid_after"`

	// the marshaled json string
	MarshaledCert []byte
}

func UnmarshalGeoCertificate(marshaledCertificate []byte) (*GeoCertificate, error) {
	certificate := new(GeoCertificate)
	err := json.Unmarshal(marshaledCertificate, certificate)
	if err != nil {
		return nil, err
	}

	certificate.MarshaledCert = marshaledCertificate

	return certificate, nil
}

// returns the marshaled certificate
func (cert *GeoCertificate) Marshal() []byte {
	return cert.MarshaledCert
}

func (cert *GeoCertificate) Hash() SHA256Hash {
	hash := sha256.Sum256(cert.MarshaledCert)
	return hash[:]
}

func (cert *GeoCertificate) BitStrings(fGrow float64) ([]*bitstring.RawBitStringPair, error) {
	bitstrings := make([]*bitstring.RawBitStringPair, 0)

	for i, area := range cert.Areas {
		altitude := cert.AreasAltitude[i]
		altitudeMin := altitude[0]
		altitudeMax := altitude[1]

		loops, err := area.Loops()

		if err != nil {
			return nil, err
		}

		bs, err := bitstring.ExtrudedPolygonsToBitStringPairs(loops, altitudeMin, altitudeMax, fGrow)
		if err != nil {
			return nil, err
		}

		bitstrings = append(bitstrings, bs...)
	}

	return bitstrings, nil
}
