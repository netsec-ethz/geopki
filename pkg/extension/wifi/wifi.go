// Package wifi defines the "wifi" GeoCertificate extension: a WiFi network
// registered for the certificate's areas. See README.md in this directory
// for more information.

package wifi

import (
	"bytes"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"

	"geopki/pkg/extension"
)

// Name is the extension's name in crypto.GeoCertificate.Extensions.
const Name = "wifi"

// Version is the version of the format in this package.
const Version = 1

func init() {
	extension.Register(Name, Validate)
}

// AuthMode is how a client authenticates to the network. Values not listed
// below are allowed: a client treats an unknown mode as matching nothing.
type AuthMode string

const (
	AuthOpen              AuthMode = "open"
	AuthOWE               AuthMode = "owe"
	AuthWPA2Personal      AuthMode = "wpa2-personal"
	AuthWPA3Personal      AuthMode = "wpa3-personal"
	AuthWPA2Enterprise    AuthMode = "wpa2-enterprise"
	AuthWPA3Enterprise    AuthMode = "wpa3-enterprise"
	AuthWPA3Enterprise192 AuthMode = "wpa3-enterprise-192"
)

// IsEnterprise reports whether the mode authenticates with 802.1X / EAP.
func (mode AuthMode) IsEnterprise() bool {
	return mode == AuthWPA2Enterprise || mode == AuthWPA3Enterprise || mode == AuthWPA3Enterprise192
}

// The extension: one WiFi network per certificate.
type WiFi struct {
	// format version, see Version
	Version int `json:"version"`
	// display name of the network's operator, never used for matching
	Operator string `json:"operator,omitempty"`

	SSID string `json:"ssid"`
	// false: the SSID is exclusive to this operator in the certificate's areas
	SSIDShared bool `json:"ssid_shared,omitempty"`
	// every mode the network advertises
	AuthModes []AuthMode `json:"auth_modes"`
	// "optional", "required", or empty if unspecified
	PMF string `json:"pmf,omitempty"`

	// only required for the enterprise modes, otherwise absent
	Enterprise *Enterprise `json:"enterprise,omitempty"`
	// the network's captive portal (if present)
	Portal *Portal `json:"portal,omitempty"`
}

// Enterprise describes the authentication of a WPA2/WPA3-Enterprise network.
type Enterprise struct {
	// name of a roaming federation the site belongs to (empty for closed network)
	Federation string `json:"federation,omitempty"`
	// realm groups of the local users, may be empty only in a federation
	// (a site without local users, where everyone is a visitor)
	AAA []AAADomain `json:"aaa"`
}

// AAADomain is one set of users and the servers that authenticate them.
type AAADomain struct {
	// NAI realms: "corp.example" matches exactly, "*.corp.example" subrealms only
	Realms     []string     `json:"realms,omitempty"`
	EAPMethods []EAPMethod  `json:"eap_methods,omitempty"`
	Servers    []AuthServer `json:"servers"`
}

// EAPMethod is an EAP method ("eap-tls", "peap", "ttls") with its inner methods.
type EAPMethod struct {
	Method string   `json:"method"`
	Phase2 []string `json:"phase2,omitempty"`
}

// AuthServer is a RADIUS server, identified by its certificate.
type AuthServer struct {
	// SAN dNSName (CN if none) of the server certificate
	Names []string `json:"names,omitempty"`
	// SHA-256 of the leaf certificate's SPKI, base64
	PinnedSPKIHashes []string `json:"pinned_spki_sha256,omitempty"`
	// SHA-256 of a CA's SPKI, base64
	CASPKIHashes []string `json:"ca_spki_sha256,omitempty"`
	// the CA certificates themselves, base64 DER, for clients that have to
	// pin the CA inside the handshake
	CACertificates []string `json:"ca_certificates,omitempty"`
}

// Portal is the HTTPS identity of a captive portal.
type Portal struct {
	Domains []PortalDomain `json:"domains"`
}

// PortalDomain is one host of the portal. Role "primary" is the entry point,
// "delegate" a page only reached after it (e.g. payment), "api" the host of
// the Captive Portal API (RFC 8908).
type PortalDomain struct {
	Name                    string   `json:"name"`
	Role                    string   `json:"role"`
	PinnedSPKIHashes        []string `json:"pinned_spki_sha256"`
	CertificateFingerprints []string `json:"certificate_sha256,omitempty"`
}

// Parse reads and validates the extension.
func Parse(raw json.RawMessage) (*WiFi, error) {
	// encoding/json matches keys case-insensitively and keeps the last of
	// duplicate keys, but clients may not: reject both, so every reader sees
	// the same certificate
	if err := checkKeys(raw); err != nil {
		return nil, err
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	wifi := new(WiFi)
	if err := decoder.Decode(wifi); err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, fmt.Errorf("trailing data after the extension")
	}

	if err := wifi.validate(); err != nil {
		return nil, err
	}
	return wifi, nil
}

// Validate is the extension's validator (see geopki/pkg/extension).
func Validate(raw json.RawMessage) error {
	_, err := Parse(raw)
	return err
}

func (wifi *WiFi) validate() error {
	if wifi.Version != Version {
		return fmt.Errorf("unsupported version %d, expected %d", wifi.Version, Version)
	}
	if wifi.SSID == "" {
		return fmt.Errorf("ssid is missing")
	}
	// IEEE 802.11: an SSID is at most 32 octets
	if len(wifi.SSID) > 32 {
		return fmt.Errorf("ssid is longer than 32 bytes")
	}
	if len(wifi.AuthModes) == 0 {
		return fmt.Errorf("auth_modes is empty")
	}
	if wifi.PMF != "" && wifi.PMF != "optional" && wifi.PMF != "required" {
		return fmt.Errorf("pmf must be \"optional\", \"required\" or absent, not %q", wifi.PMF)
	}

	enterpriseMode := false
	for _, mode := range wifi.AuthModes {
		enterpriseMode = enterpriseMode || mode.IsEnterprise()
	}
	switch {
	case enterpriseMode && wifi.Enterprise == nil:
		return fmt.Errorf("enterprise auth modes without an enterprise block")
	case !enterpriseMode && wifi.Enterprise != nil:
		return fmt.Errorf("enterprise block without an enterprise auth mode")
	}

	if wifi.Enterprise != nil {
		if err := wifi.Enterprise.validate(); err != nil {
			return fmt.Errorf("enterprise: %v", err)
		}
	}
	if wifi.Portal != nil {
		if err := wifi.Portal.validate(); err != nil {
			return fmt.Errorf("portal: %v", err)
		}
	}
	return nil
}

func (enterprise *Enterprise) validate() error {
	federated := enterprise.Federation != ""
	if len(enterprise.AAA) == 0 && !federated {
		return fmt.Errorf("aaa is empty in a closed network")
	}
	for i, aaa := range enterprise.AAA {
		if federated && len(aaa.Realms) == 0 {
			return fmt.Errorf("aaa[%d]: realms are required in a federation", i)
		}
		for _, realm := range aaa.Realms {
			if realm == "" || realm == "*" || strings.Contains(strings.TrimPrefix(realm, "*."), "*") {
				return fmt.Errorf("aaa[%d]: invalid realm %q", i, realm)
			}
		}
		if len(aaa.Servers) == 0 {
			return fmt.Errorf("aaa[%d]: servers is empty", i)
		}
		for j, server := range aaa.Servers {
			if err := server.validate(); err != nil {
				return fmt.Errorf("aaa[%d].servers[%d]: %v", i, j, err)
			}
		}
	}
	return nil
}

func (server *AuthServer) validate() error {
	if len(server.PinnedSPKIHashes) == 0 && len(server.CASPKIHashes) == 0 {
		return fmt.Errorf("no pins: needs pinned_spki_sha256 or ca_spki_sha256")
	}
	// a CA pin vouches for every server the CA issued for; only the names tie it to this one
	if len(server.CASPKIHashes) > 0 && len(server.Names) == 0 {
		return fmt.Errorf("ca_spki_sha256 without names")
	}
	if err := checkHashes("pinned_spki_sha256", server.PinnedSPKIHashes); err != nil {
		return err
	}
	if err := checkHashes("ca_spki_sha256", server.CASPKIHashes); err != nil {
		return err
	}
	for i, encoded := range server.CACertificates {
		der, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return fmt.Errorf("ca_certificates[%d]: not base64: %v", i, err)
		}
		if _, err := x509.ParseCertificate(der); err != nil {
			return fmt.Errorf("ca_certificates[%d]: not a DER certificate: %v", i, err)
		}
	}
	return nil
}

func (portal *Portal) validate() error {
	if len(portal.Domains) == 0 {
		return fmt.Errorf("domains is empty")
	}
	for i, domain := range portal.Domains {
		if domain.Name == "" {
			return fmt.Errorf("domains[%d]: name is missing", i)
		}
		if domain.Role == "" {
			return fmt.Errorf("domains[%d]: role is missing", i)
		}
		// a client does no Web PKI validation, so a domain without pins could never be verified
		if len(domain.PinnedSPKIHashes) == 0 {
			return fmt.Errorf("domains[%d] (%s): pinned_spki_sha256 is empty", i, domain.Name)
		}
		if err := checkHashes("pinned_spki_sha256", domain.PinnedSPKIHashes); err != nil {
			return fmt.Errorf("domains[%d] (%s): %v", i, domain.Name, err)
		}
		if err := checkHashes("certificate_sha256", domain.CertificateFingerprints); err != nil {
			return fmt.Errorf("domains[%d] (%s): %v", i, domain.Name, err)
		}
	}
	return nil
}

// checkHashes requires every entry to be a base64 SHA-256 digest.
func checkHashes(field string, hashes []string) error {
	for _, encoded := range hashes {
		digest, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(digest) != 32 {
			return fmt.Errorf("%s: %q is not a base64 SHA-256 digest", field, encoded)
		}
	}
	return nil
}

// checkKeys rejects duplicate keys and keys that aren't lowercase snake_case
// anywhere in raw. Together with DisallowUnknownFields, which rejects names
// the format doesn't have, only exact keys remain.
func checkKeys(raw json.RawMessage) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	return walk(decoder, "")
}

// walk consumes one JSON value from decoder; path is where it is, for errors.
func walk(decoder *json.Decoder, path string) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}

	switch delim {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key := keyToken.(string)
			if seen[key] {
				return fmt.Errorf("duplicate key %q at %s", key, pathOrRoot(path))
			}
			seen[key] = true
			if !isSnakeCase(key) {
				return fmt.Errorf("key %q at %s is not lowercase snake_case", key, pathOrRoot(path))
			}
			if err := walk(decoder, path+"."+key); err != nil {
				return err
			}
		}
	case '[':
		for i := 0; decoder.More(); i++ {
			if err := walk(decoder, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	}
	// the closing delimiter
	_, err = decoder.Token()
	return err
}

func isSnakeCase(key string) bool {
	if key == "" {
		return false
	}
	for _, r := range key {
		if !(unicode.IsLower(r) || unicode.IsDigit(r) || r == '_') || r > unicode.MaxASCII {
			return false
		}
	}
	return true
}

func pathOrRoot(path string) string {
	if path == "" {
		return "the top level"
	}
	return strings.TrimPrefix(path, ".")
}
