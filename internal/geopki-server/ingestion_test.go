package server

import (
	"crypto/sha256"
	"strings"
	"testing"
)

const area = `"areas":[{"type":"MultiPolygon","coordinates":[[[[8.5,47.3],[8.6,47.3],[8.6,47.4],[8.5,47.4],[8.5,47.3]]],[]]}],"areas_altitude":[[0,500]],"not_valid_after":"2030-01-01T00:00:00Z"`

// One insertion with a certificate of each kind: a plain one as in the
// address dataset, one with an extension the server doesn't know, and a WiFi
// certificate. All keep the bytes they were submitted with.
func TestParseCertificatesKeepsSubmittedBytes(t *testing.T) {
	plain := `{"certificate_id":"shop","domain":"shop.example",` + area + `}`
	unknown := `{ "certificate_id": "atm", "domain": "bank.example", ` + area + `, "extensions": {"atm": {"cash": true}} }`
	wifi := `{"certificate_id":"cafe-wifi","domain":"cafe.example",` + area + `,"critical":["wifi"],"extensions":{"wifi":{"version":1,"ssid":"cafe","auth_modes":["open"]}}}`

	certificates, err := parseCertificates([]byte("[" + plain + ",\n" + unknown + "," + wifi + "]"))
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{plain, unknown, wifi} {
		if got := string(certificates[i].Marshal()); got != want {
			t.Errorf("certificate %d: stored bytes differ from the submitted ones:\n got  %s\n want %s", i, got, want)
		}
		if certificates[i].Hash() != sha256.Sum256([]byte(want)) {
			t.Errorf("certificate %d: hash is not over the submitted bytes", i)
		}
	}
}

func TestParseCertificatesRejects(t *testing.T) {
	cases := map[string]struct{ body, want string }{
		"not an array":           {`{"certificate_id":"x"}`, "cannot unmarshal"},
		"invalid wifi extension": {`[{"certificate_id":"w","domain":"d",` + area + `,"extensions":{"wifi":{"version":1,"ssid":"","auth_modes":["open"]}}}]`, "ssid is missing"},
		"critical but missing":   {`[{"certificate_id":"w","domain":"d",` + area + `,"critical":["wifi"]}]`, `critical extension "wifi" is missing`},
		"altitudes don't match":  {`[{"certificate_id":"w","domain":"d","areas":[{"type":"MultiPolygon"}],"areas_altitude":[],"not_valid_after":"2030-01-01T00:00:00Z"}]`, "altitude ranges"},
	}
	for name, c := range cases {
		_, err := parseCertificates([]byte(c.body))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want error containing %q, got %v", name, c.want, err)
		}
	}
}
