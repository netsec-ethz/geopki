package crypto

// a linear ring as defined in https://www.rfc-editor.org/rfc/rfc7946#section-3.1.6
type LinearRing = [][2]float64

// the first element is a list of exterior rings, the second one a list of interior rings
type ExteriorInteriorRings = [2][]LinearRing

type GeoCertArea struct {
	Coordinates ExteriorInteriorRings `json:"coordinates"`
	Type        string                `json:"type"`
}

// this is a mock certificate format until proper x509 extensions are defined
type GeoCertificate struct {
	Certificate_id string        `json:"certificate_id"`
	Domain         string        `json:"domain"`
	Levels         []string      `json:"levels"`
	Area           []GeoCertArea `json:"area"`
}
