package crypto

import (
	"crypto/sha256"
	"testing"
)

// DEFAULT_HASH is hardcoded as a byte literal in hash.go, where it is documented
// as the default SMT hash value, i.e. SHA256(0x00). Pin the literal to the digest
// it claims to be, so a typo or an accidental edit cannot silently change the
// default that the whole sparse Merkle tree is built on.
func TestDefaultHashIsSha256OfZeroByte(t *testing.T) {
	expected := sha256.Sum256([]byte{0x00})

	if DEFAULT_HASH != expected {
		t.Fatalf(`DEFAULT_HASH should be SHA256(0x00) = %x, got %x`, expected[:], DEFAULT_HASH[:])
	}
}
