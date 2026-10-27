package extension

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"geopki/pkg/crypto"
)

func init() {
	// a test extension: valid when its JSON is the object {"ok":true}
	Register("test-ext", func(raw json.RawMessage) error {
		var content struct{ Ok bool }
		if err := json.Unmarshal(raw, &content); err != nil || !content.Ok {
			return fmt.Errorf("not ok")
		}
		return nil
	})
}

func certificate(critical []string, extensions map[string]string) *crypto.GeoCertificate {
	raw := map[string]json.RawMessage{}
	for name, content := range extensions {
		raw[name] = json.RawMessage(content)
	}
	return &crypto.GeoCertificate{CertificateId: "c1", Critical: critical, Extensions: raw}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name       string
		critical   []string
		extensions map[string]string
		wantError  string
	}{
		{"no extensions", nil, nil, ""},
		{"registered and valid", []string{"test-ext"}, map[string]string{"test-ext": `{"ok":true}`}, ""},
		{"registered and invalid", nil, map[string]string{"test-ext": `{"ok":false}`}, `extension "test-ext": not ok`},
		// the server stores what it can't judge; clients that use the extension judge it
		{"unknown extension", []string{"unknown"}, map[string]string{"unknown": `{"anything":1}`}, ""},
		{"critical but missing", []string{"test-ext"}, nil, `critical extension "test-ext" is missing`},
		{"critical twice", []string{"test-ext", "test-ext"}, map[string]string{"test-ext": `{"ok":true}`}, "listed as critical twice"},
	}

	for _, c := range cases {
		err := Validate(certificate(c.critical, c.extensions))
		switch {
		case c.wantError == "" && err != nil:
			t.Errorf("%s: unexpected error %v", c.name, err)
		case c.wantError != "" && (err == nil || !strings.Contains(err.Error(), c.wantError)):
			t.Errorf("%s: want error containing %q, got %v", c.name, c.wantError, err)
		}
	}
}

func TestRegisterTwicePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Errorf("registering a name twice didn't panic")
		}
	}()
	Register("test-ext", func(json.RawMessage) error { return nil })
}

func TestRegistered(t *testing.T) {
	found := false
	for _, name := range Registered() {
		found = found || name == "test-ext"
	}
	if !found {
		t.Errorf("Registered() = %v, missing test-ext", Registered())
	}
}
