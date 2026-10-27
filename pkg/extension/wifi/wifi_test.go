package wifi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"geopki/pkg/crypto"
	"geopki/pkg/extension"
)

// loads the certificates of a file in testdata (an insertion request: a JSON array)
func loadCertificates(t *testing.T, file string) []*crypto.GeoCertificate {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", file))
	if err != nil {
		t.Fatal(err)
	}
	var raws []json.RawMessage
	if err := json.Unmarshal(body, &raws); err != nil {
		t.Fatal(err)
	}
	certificates := make([]*crypto.GeoCertificate, len(raws))
	for i, raw := range raws {
		if certificates[i], err = crypto.UnmarshalGeoCertificate(raw); err != nil {
			t.Fatal(err)
		}
	}
	return certificates
}

// An open network with a captive portal, a site of a roaming federation
// without local users, and a closed enterprise network pinned to its CA.
func TestTestdataIsValid(t *testing.T) {
	for _, file := range []string{"open-portal.json", "federation-site.json", "enterprise-ca.json"} {
		for _, certificate := range loadCertificates(t, file) {
			if err := certificate.CheckStructure(); err != nil {
				t.Errorf("%s: %v", file, err)
			}
			// through the registry, as the server does on insertion
			if err := extension.Validate(certificate); err != nil {
				t.Errorf("%s: %v", file, err)
			}
			if _, err := Parse(certificate.Extensions[Name]); err != nil {
				t.Errorf("%s: %v", file, err)
			}
		}
	}
}

func TestParse(t *testing.T) {
	wifi, err := Parse(loadCertificates(t, "enterprise-ca.json")[0].Extensions[Name])
	if err != nil {
		t.Fatal(err)
	}
	if wifi.SSID != "Corp Staff" || wifi.Enterprise == nil || wifi.Enterprise.Federation != "" {
		t.Errorf("unexpected content: %+v", wifi)
	}
	if len(wifi.Enterprise.AAA) == 0 || len(wifi.Enterprise.AAA[0].Servers) == 0 ||
		len(wifi.Enterprise.AAA[0].Servers[0].CACertificates) == 0 {
		t.Errorf("CA certificates were not read")
	}
}

const hash = "UNhY4JhezH9gQYqvDMWrWH9CwlcKiECVqejMrND2VFw=" // a base64 SHA-256 digest

// an enterprise network that passes; each case below breaks one rule
const closedEnterprise = `{"version":1,"ssid":"corp","auth_modes":["wpa2-enterprise"],
  "enterprise":{"aaa":[{"servers":[{"names":["radius.corp.example"],"ca_spki_sha256":["` + hash + `"]}]}]}}`

func TestRules(t *testing.T) {
	if err := Validate(json.RawMessage(closedEnterprise)); err != nil {
		t.Fatalf("valid enterprise network rejected: %v", err)
	}

	open := func(rest string) string {
		return `{"version":1,"ssid":"cafe","auth_modes":["open"]` + rest + `}`
	}
	portal := func(domain string) string { return open(`,"portal":{"domains":[` + domain + `]}`) }
	enterprise := func(aaa string) string {
		return `{"version":1,"ssid":"corp","auth_modes":["wpa2-enterprise"],"enterprise":` + aaa + `}`
	}
	server := func(fields string) string { return enterprise(`{"aaa":[{"servers":[{` + fields + `}]}]}`) }

	accepted := map[string]string{
		// an auth mode this version doesn't know matches nothing on the client; not the server's call
		"unknown auth mode": `{"version":1,"ssid":"cafe","auth_modes":["wpa4-something"]}`,
		"portal":            portal(`{"name":"portal.cafe.example","role":"primary","pinned_spki_sha256":["` + hash + `"]}`),
		"federation site":   enterprise(`{"federation":"examplefed","aaa":[]}`),
		"leaf pin, no name": server(`"pinned_spki_sha256":["` + hash + `"]`),
		"pmf":               open(`,"pmf":"required"`),
	}
	for name, raw := range accepted {
		if err := Validate(json.RawMessage(raw)); err != nil {
			t.Errorf("%s: rejected: %v", name, err)
		}
	}

	rejected := map[string]struct{ raw, want string }{
		"unknown key":                     {open(`,"colour":"blue"`), "unknown field"},
		"miscased key":                    {`{"version":1,"SSID":"cafe","auth_modes":["open"]}`, "snake_case"},
		"duplicate key":                   {`{"version":1,"ssid":"a","ssid":"b","auth_modes":["open"]}`, "duplicate key"},
		"nested duplicate":                {portal(`{"name":"a","name":"b","role":"primary","pinned_spki_sha256":["` + hash + `"]}`), "duplicate key"},
		"other version":                   {`{"version":2,"ssid":"cafe","auth_modes":["open"]}`, "unsupported version"},
		"no ssid":                         {`{"version":1,"auth_modes":["open"]}`, "ssid is missing"},
		"long ssid":                       {`{"version":1,"ssid":"` + strings.Repeat("x", 33) + `","auth_modes":["open"]}`, "32 bytes"},
		"no auth modes":                   {`{"version":1,"ssid":"cafe","auth_modes":[]}`, "auth_modes is empty"},
		"bad pmf":                         {open(`,"pmf":"sometimes"`), "pmf"},
		"enterprise mode without block":   {`{"version":1,"ssid":"corp","auth_modes":["wpa3-enterprise"]}`, "without an enterprise block"},
		"block without enterprise mode":   {open(`,"enterprise":{"federation":"examplefed","aaa":[]}`), "without an enterprise auth mode"},
		"closed network without aaa":      {enterprise(`{"aaa":[]}`), "closed network"},
		"federation entry without realms": {enterprise(`{"federation":"examplefed","aaa":[{"servers":[{"pinned_spki_sha256":["` + hash + `"]}]}]}`), "realms are required"},
		"invalid realm":                   {enterprise(`{"aaa":[{"realms":["a.*.b"],"servers":[{"pinned_spki_sha256":["` + hash + `"]}]}]}`), "invalid realm"},
		"no servers":                      {enterprise(`{"aaa":[{"servers":[]}]}`), "servers is empty"},
		"server without pins":             {server(`"names":["radius.corp.example"]`), "no pins"},
		"CA pin without names":            {server(`"ca_spki_sha256":["` + hash + `"]`), "without names"},
		"short hash":                      {server(`"pinned_spki_sha256":["AAAA"]`), "not a base64 SHA-256"},
		"CA certificate not DER":          {server(`"names":["r"],"ca_spki_sha256":["` + hash + `"],"ca_certificates":["AAAA"]`), "not a DER certificate"},
		"portal domain without pins":      {portal(`{"name":"portal.cafe.example","role":"primary","pinned_spki_sha256":[]}`), "pinned_spki_sha256 is empty"},
		"portal domain without role":      {portal(`{"name":"portal.cafe.example","pinned_spki_sha256":["` + hash + `"]}`), "role is missing"},
		"empty portal":                    {open(`,"portal":{"domains":[]}`), "domains is empty"},
		"trailing data":                   {open(``) + `{}`, "trailing data"},
	}
	for name, c := range rejected {
		err := Validate(json.RawMessage(c.raw))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want error containing %q, got %v", name, c.want, err)
		}
	}
}

func TestRegisteredWithTheServerRegistry(t *testing.T) {
	for _, name := range extension.Registered() {
		if name == Name {
			return
		}
	}
	t.Errorf("%q is not registered: %v", Name, extension.Registered())
}
