package crypto

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"

	"geopki/pkg/bitstring"
	"geopki/pkg/comm"

	mapset "github.com/deckarep/golang-set/v2"
)

// verifies a recieved response based on a public key
// and returns the set of all certificate hashes as hex strings
func VerifyResponse(response *comm.Response, publicKey *ecdsa.PublicKey) (mapset.Set[string], error) {
	smh := NewSMHFromCommSMH(response.GetSignedMapHead())

	if !smh.Verify(publicKey) {
		return nil, fmt.Errorf("signature on the SMH is invalid")
	}

	ns := response.GetNodes()
	nodes := make([]*Node, len(ns))

	bitStringMap := make(map[bitstring.RawBitStringPair]*Node)

	var rootNode *Node

	for i, n := range ns {

		if n.XYBitStringLen > uint32(bitstring.XY_BITS) {
			return nil, fmt.Errorf("received xyBitStringLen is greater than XY_BITS")
		}

		if n.ZBitString > math.MaxUint16 {
			return nil, fmt.Errorf("received invalid ZBitString")
		}

		if n.ZBitStringLen > uint32(bitstring.Z_BITS) {
			return nil, fmt.Errorf("received zBitStringLen is greater than Z_BITS")
		}

		// check if bit strings are properly formatted
		// by ANDing with mask to filter out only the bits that should be cleared
		if (n.XYBitString & (uint64(math.MaxUint64) >> n.XYBitStringLen)) != 0 {
			return nil, fmt.Errorf("received invalid xy bit string, the lower bits are not all cleared for ")
		}

		// unfortunately protobufs do not support uint16 directly, two MSBs are unused
		zBitString := uint16(n.ZBitString)
		if (zBitString & (uint16(math.MaxUint16) >> n.ZBitStringLen)) != 0 {
			return nil, fmt.Errorf("received invalid z bit string, the lower bits are not all cleared")
		}

		node := NewTreeNode(
			n.XYBitString,
			uint8(n.XYBitStringLen),
			zBitString,
			uint8(n.ZBitStringLen),
			n.GetXYLeftChildHash(),
			n.GetXYRightChildHash(),
			n.GetZLeftChildHash(),
			n.GetZRightChildHash(),
			n.GetCertificateHashes(),
		)

		_, ok := bitStringMap[node.RawBitStringPair]
		if ok {
			return nil, fmt.Errorf("received two nodes with the same bit string")
		}

		bitStringMap[node.RawBitStringPair] = node
		nodes[i] = node

		if node.IsRoot() {
			rootNode = node
		}
	}

	if rootNode == nil {
		return nil, fmt.Errorf("response did not contain the root node")
	}

	// build the tree
	for _, node := range nodes {

		if node.neighborHash == nil {
			neighbor, ok := bitStringMap[node.NeighborPair()]

			if ok {
				node.SetNeighbor(neighbor)
			}

			// else: response did not contain neighbor nor neighbor hash
			// -> must be default hash -> set nothing

		}

		if node.xyLeftChildHash == nil {
			childBitString, err := node.XYLeftChildPair()
			// if err == nil child does not exist -> default hash -> do nothing

			if err == nil {
				child, ok := bitStringMap[childBitString]

				if ok {
					node.SetXYLeftChild(child)
				}

				// else: response did not contain child nor child hash
				// -> must be default hash -> set nothing
			}

		}

		if node.xyRightChildHash == nil {
			childBitString, err := node.XYRightChildPair()
			// if err == nil child does not exist -> default hash -> do nothing

			if err == nil {
				child, ok := bitStringMap[childBitString]

				if ok {
					node.SetXYRightChild(child)
				}

				// else: response did not contain child nor child hash
				// -> must be default hash -> set nothing
			}

		}

		if node.zLeftChildHash == nil {
			child, ok := bitStringMap[node.ZLeftChildPair()]

			if ok {
				node.SetZLeftChild(child)
			}

			// else: response did not contain child nor child hash
			// -> must be default hash -> set nothing

		}

		if node.zRightChildHash == nil {
			child, ok := bitStringMap[node.ZRightChildPair()]

			if ok {
				node.SetZRightChild(child)
			}

			// else: response did not contain child nor child hash
			// -> must be default hash -> set nothing

		}
	}

	// ensure all nodes are in the tree now
	if len(ns) != rootNode.CountNodes() {
		return nil, fmt.Errorf("received invalid tree, cannot use all nodes in tree. built tree contains %d nodes but received %d nodes", rootNode.CountNodes(), len(ns))
	}

	// compute the root hash
	rootHash := rootNode.Hash()

	// verify root hash against SMH
	if !bytes.Equal(rootHash, response.SignedMapHead.RootHash) {
		return nil, fmt.Errorf("computed root hash does not match the SMH")
	}

	// verify that all received certificate's hash is in one of the nodes

	// first compute the set of all certificate hashes
	// unfortunately in string form since []byte is not comparable
	certificateStringHashes := mapset.NewSet[string]()
	for _, node := range response.Nodes {
		for _, certificateHash := range node.CertificateHashes {
			certificateStringHashes.Add(hex.EncodeToString(certificateHash))
		}
	}

	for _, certificate := range response.GetCertificates() {
		hash := sha256.Sum256(certificate)
		hashString := hex.EncodeToString(hash[:])

		if !certificateStringHashes.Contains(hashString) {
			return nil, fmt.Errorf("certificate with hash '%s' is part of the response but is not contained in any node", hashString)
		}
	}

	// verification succeeded, return certificates
	return certificateStringHashes, nil
}
