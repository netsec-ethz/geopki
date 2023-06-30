package crypto

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"geopki/pkg/bitstring"

	mapset "github.com/deckarep/golang-set/v2"
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

		// array of ccw order points of the loop
		pts := make([]s2.Point, 0, len(polygon))
		// also keep a set of the points to check for duplicate vertices
		certificateStringHashes := mapset.NewThreadUnsafeSet[s2.Point]()

		for i, pt := range polygon {
			// geojson is (longitude, latitude), see https://datatracker.ietf.org/doc/html/rfc7946#section-3.1.1
			p := s2.PointFromLatLng(s2.LatLngFromDegrees(pt[1], pt[0]))

			if i == 0 || !pts[len(pts)-1].ApproxEqual(p) {
				if certificateStringHashes.Contains(p) {
					// Loops are not allowed to have any duplicate vertices (whether adjacent or not).
					// https://pkg.go.dev/github.com/golang/geo@v0.0.0-20230421003525-6adc56603217/s2#Loop
					return nil, fmt.Errorf("loop contains duplicate vertices")
				}

				// add point to the polygon and the set for duplicate checks
				pts = append(pts, p)
				certificateStringHashes.Add(p)
			}
			// otherwise skip this point, equal to the last one -> fix loop
		}

		loops[i] = s2.LoopFromPoints(pts)

		if loops[i].Area() > 12 {
			// fmt.Printf("Warning: changing order of polygon: %s\n", bitstring.LoopToGeoJson(loops[i]))

			// this loop covers almost the whole world, is likely not right-hand rule sorted
			// -> reverse
			for i, j := 0, len(pts)-1; i < j; i, j = i+1, j-1 {
				pts[i], pts[j] = pts[j], pts[i]
			}

			loops[i] = s2.LoopFromPoints(pts)
		}

		err := loops[i].Validate()
		if err != nil {
			fmt.Printf("%s\n", bitstring.LoopToGeoJsonFeature(loops[i]))
			return nil, fmt.Errorf("created invalid loop: %v", err)
		}
	}

	return loops, nil
}

// this is a mock certificate format until proper x509 extensions are defined
type GeoCertificate struct {
	CertificateId string `json:"certificate_id"`
	Domain        string `json:"domain"`

	// a list of areas associated with this certificate
	Areas []GeoCertArea `json:"areas"`
	// for each of the areas the minimum and maximum altitudes
	AreasAltitude [][2]float64 `json:"areas_altitude"`

	NotValidAfter string `json:"not_valid_after"`

	// the marshaled json string
	MarshaledCert []byte `json:"-"`
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

// returns a version of the certificate
func (cert *GeoCertificate) JSON() string {
	return string(cert.MarshaledCert)
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

		polygons := make([]bitstring.Geometry2D, len(loops))
		for i, loop := range loops {
			polygons[i] = &bitstring.S2Geometry2D{
				Loop: loop,
			}
		}

		bs, err := bitstring.ExtrudedPolygonsToBitStringPairs(polygons, altitudeMin, altitudeMax, fGrow)
		if err != nil {
			return nil, err
		}

		bitstrings = append(bitstrings, bs...)
	}

	return bitstrings, nil
}
