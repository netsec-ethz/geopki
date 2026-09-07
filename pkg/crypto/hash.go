package crypto

// a SHA-256 hash. stored by value as a fixed-size array to reduce memory consumption.
// Relevant when millions of hashes are kept in memory (e.g. the db-address-exporter).
type SHA256Hash = [32]byte

// the default SMT hash value, i.e. SHA256(0x00)
var DEFAULT_HASH = SHA256Hash{
	0x6e, 0x34, 0x0b, 0x9c, 0xff, 0xb3, 0x7a, 0x98,
	0x9c, 0xa5, 0x44, 0xe6, 0xbb, 0x78, 0x0a, 0x2c,
	0x78, 0x90, 0x1d, 0x3f, 0xb3, 0x37, 0x38, 0x76,
	0x85, 0x11, 0xa3, 0x06, 0x17, 0xaf, 0xa0, 0x1d,
}

// BytesToHash copies b into a fixed-size hash. Extra bytes are ignored and
// missing bytes are left as zero, so it never panics on a short/long input.
func BytesToHash(b []byte) SHA256Hash {
	var h SHA256Hash
	copy(h[:], b)
	return h
}

// BytesToHashPtr returns nil for an absent (nil or empty) hash, otherwise a
// pointer to the copied hash. Preserves the "hash is absent" meaning that used
// to be carried by a nil []byte.
func BytesToHashPtr(b []byte) *SHA256Hash {
	if len(b) == 0 {
		return nil
	}
	h := BytesToHash(b)
	return &h
}

// HashPtrToBytes returns nil for an absent hash, otherwise its 32 bytes.
func HashPtrToBytes(h *SHA256Hash) []byte {
	if h == nil {
		return nil
	}
	return h[:]
}

// BytesSliceToHashes copies a slice of byte slices into a slice of hashes.
func BytesSliceToHashes(bs [][]byte) []SHA256Hash {
	if bs == nil {
		return nil
	}
	hs := make([]SHA256Hash, len(bs))
	for i, b := range bs {
		hs[i] = BytesToHash(b)
	}
	return hs
}

// HashesToBytesSlice exposes a slice of hashes as a slice of byte slices.
// The returned slices alias the input's backing array and must not be mutated.
func HashesToBytesSlice(hs []SHA256Hash) [][]byte {
	if hs == nil {
		return nil
	}
	bs := make([][]byte, len(hs))
	for i := range hs {
		bs[i] = hs[i][:]
	}
	return bs
}
