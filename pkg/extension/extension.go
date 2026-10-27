// Package extension validates the extensions of GeoCertificates.
//
// A GeoCertificate binds a domain to areas; what a certificate says beyond
// that lives in named extensions (crypto.GeoCertificate.Extensions), much like
// x509 extensions. The map server treats all certificates alike: it stores and
// proves every certificate of an area, so that clients can verify that none was
// withheld, and each client uses the extensions it understands.
//
// Packages for a particular kind of certificate (e.g. extension/wifi) register
// a validator for their extension in an init function. The server imports them
// and checks, on insertion, every extension it has a validator for. Extensions
// without a registered validator are stored as they are: the server is a log
// of what issuers submitted, and judging an extension it doesn't know is up to
// the clients that use it.
package extension

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"geopki/pkg/crypto"
)

// Validator checks the raw JSON of one extension.
type Validator func(raw json.RawMessage) error

var (
	validatorsLock sync.RWMutex
	validators     = map[string]Validator{}
)

// Register adds the validator for the extension called name. Meant to be
// called from the init function of the extension's package; registering a
// name twice panics.
func Register(name string, validator Validator) {
	validatorsLock.Lock()
	defer validatorsLock.Unlock()

	if _, exists := validators[name]; exists {
		panic(fmt.Sprintf("extension %q registered twice", name))
	}
	validators[name] = validator
}

// Registered returns the names of the extensions with a validator, sorted.
func Registered() []string {
	validatorsLock.RLock()
	defer validatorsLock.RUnlock()

	names := make([]string, 0, len(validators))
	for name := range validators {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Validate checks the extensions of a certificate:
//   - every name in Critical refers to an extension the certificate contains,
//     and appears only once
//   - every extension with a registered validator passes it
func Validate(certificate *crypto.GeoCertificate) error {
	seen := make(map[string]bool, len(certificate.Critical))
	for _, name := range certificate.Critical {
		if seen[name] {
			return fmt.Errorf("certificate %s: extension %q listed as critical twice", certificate.CertificateId, name)
		}
		seen[name] = true

		if !certificate.HasExtension(name) {
			return fmt.Errorf("certificate %s: critical extension %q is missing", certificate.CertificateId, name)
		}
	}

	validatorsLock.RLock()
	defer validatorsLock.RUnlock()

	// sorted, so that the first error is the same on every run
	names := make([]string, 0, len(certificate.Extensions))
	for name := range certificate.Extensions {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		validator, known := validators[name]
		if !known {
			continue
		}
		if err := validator(certificate.Extensions[name]); err != nil {
			return fmt.Errorf("certificate %s: extension %q: %v", certificate.CertificateId, name, err)
		}
	}

	return nil
}
