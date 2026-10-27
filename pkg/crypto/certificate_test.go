package crypto

import (
	"crypto/sha256"
	"encoding/json"
	"strings"
	"testing"
)

const certificateWithExtension = `{"certificate_id":"c1","domain":"example.com",
  "areas":[{"type":"MultiPolygon","coordinates":[[[[8.5,47.3],[8.6,47.3],[8.6,47.4],[8.5,47.4],[8.5,47.3]]],[]]}],
  "areas_altitude":[[0,500]],"not_valid_after":"2030-01-01T00:00:00Z",
  "critical":["shop"],"extensions":{"shop":{"opening_hours":"Mo-Fr 09:00-18:00","unknown_to_geopki":true}}}`

// The hash covers the submitted bytes, so issuer and clients compute the same
// hash, and fields the struct doesn't know survive.
func TestUnmarshalKeepsTheSubmittedBytes(t *testing.T) {
	raw := []byte(certificateWithExtension)

	certificate, err := UnmarshalGeoCertificate(raw)
	if err != nil {
		t.Fatal(err)
	}

	if string(certificate.Marshal()) != certificateWithExtension {
		t.Errorf("Marshal() changed the certificate's bytes")
	}
	if certificate.Hash() != sha256.Sum256(raw) {
		t.Errorf("Hash() is not the SHA-256 of the submitted bytes")
	}
	if !strings.Contains(string(certificate.Extensions["shop"]), `"unknown_to_geopki":true`) {
		t.Errorf("extension content was not kept: %s", certificate.Extensions["shop"])
	}
	if !certificate.HasExtension("shop") || certificate.HasExtension("wifi") {
		t.Errorf("HasExtension is wrong: %v", certificate.Extensions)
	}
}

// A certificate in the original format, without the new optional fields,
// still parses, and marshaling the struct adds none of them: the hashes of
// existing datasets (e.g. db-address-exporter's) don't change.
func TestCertificateWithoutExtensionsIsUnchanged(t *testing.T) {
	original := &GeoCertificate{
		CertificateId: "c2",
		Domain:        "example.com",
		Areas:         []GeoCertArea{{Type: "MultiPolygon"}},
		AreasAltitude: [][2]float64{{0, 100}},
		NotValidAfter: "2030-01-01T00:00:00Z",
	}

	marshaled, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"not_valid_before", "critical", "extensions"} {
		if strings.Contains(string(marshaled), field) {
			t.Errorf("empty optional field %q appears in %s", field, marshaled)
		}
	}

	parsed, err := UnmarshalGeoCertificate(marshaled)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Extensions != nil || parsed.Critical != nil {
		t.Errorf("expected no extensions, got %v / %v", parsed.Extensions, parsed.Critical)
	}
}

func TestCheckStructure(t *testing.T) {
	valid := func() *GeoCertificate {
		return &GeoCertificate{
			CertificateId: "c3",
			Areas:         []GeoCertArea{{Type: "MultiPolygon"}, {Type: "MultiPolygon"}},
			AreasAltitude: [][2]float64{{0, 100}, {0, 200}},
			NotValidAfter: "2030-01-01T00:00:00Z",
		}
	}

	if err := valid().CheckStructure(); err != nil {
		t.Errorf("valid certificate rejected: %v", err)
	}

	cases := map[string]func(*GeoCertificate){
		"no id":                       func(c *GeoCertificate) { c.CertificateId = "" },
		"no expiry":                   func(c *GeoCertificate) { c.NotValidAfter = "" },
		"no areas":                    func(c *GeoCertificate) { c.Areas, c.AreasAltitude = nil, nil },
		"altitudes don't match areas": func(c *GeoCertificate) { c.AreasAltitude = c.AreasAltitude[:1] },
	}
	for name, mutate := range cases {
		certificate := valid()
		mutate(certificate)
		if err := certificate.CheckStructure(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
