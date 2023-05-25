package database

import (
	"geopki/pkg/bitstring"
)

type SHA256Hash = []byte

type Node struct {
	// the raw bit string associated with this node
	bitstring.RawBitStringPair

	// the hashes of the neighbor, and all children
	NeighborHash, XYLeftChildHash, XYRightChildHash, ZLeftChildHash, ZRightChildHash SHA256Hash

	CertificateHashes []SHA256Hash
}

// returns XYBitString and ZBitString as a pair struct
func (node Node) Pair() bitstring.RawBitStringPair {
	return node.RawBitStringPair
}
