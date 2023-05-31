package crypto

import (
	"bytes"
	"crypto/ecdsa"
	"fmt"
	"math"

	"geopki/pkg/bitstring"
	"geopki/pkg/comm"
)

func VerifyResponse(response *comm.Response, publicKey *ecdsa.PublicKey) error {
	smh := NewSMHFromCommSMH(response.GetSignedMapHead())

	if !smh.Verify(publicKey) {
		return fmt.Errorf("signature on the SMH is invalid")
	}

	ns := response.GetNodes()
	nodes := make([]*Node, len(ns))

	bitStringMap := make(map[bitstring.RawBitStringPair]*Node)

	var rootNode *Node

	for i, n := range ns {

		if n.XYBitStringLen > uint32(bitstring.XY_BITS) {
			return fmt.Errorf("received xyBitStringLen is greater than XY_BITS")
		}

		if n.ZBitString > math.MaxUint16 {
			return fmt.Errorf("received invalid ZBitString")
		}

		if n.ZBitStringLen > uint32(bitstring.Z_BITS) {
			return fmt.Errorf("received zBitStringLen is greater than Z_BITS")
		}

		// check if bit strings are properly formatted
		// by ANDing with mask to filter out only the bits that should be cleared
		if (n.XYBitString & (uint64(math.MaxUint64) >> n.XYBitStringLen)) != 0 {
			return fmt.Errorf("received invalid xy bit string, the lower bits are not all cleared")
		}

		// unfortunately protobufs do not support uint16 directly, two MSBs are unused
		zBitString := uint16(n.ZBitString)
		if (zBitString & (uint16(math.MaxUint16) >> n.ZBitStringLen)) != 0 {
			return fmt.Errorf("received invalid z bit string, the lower bits are not all cleared")
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

		// print("received ")
		// println(node.RawBitStringPair.BitStringPair().BitStringPair())

		_, ok := bitStringMap[node.RawBitStringPair]
		if ok {
			return fmt.Errorf("received two nodes with the same bit string")
		}

		bitStringMap[node.RawBitStringPair] = node
		nodes[i] = node

		if node.IsRoot() {
			rootNode = node
		}
	}

	if rootNode == nil {
		return fmt.Errorf("response did not contain the root node")
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
		return fmt.Errorf("received invalid tree, cannot use all nodes in tree. built tree has a height of %d, received %d nodes", rootNode.CountNodes(), len(ns))
	}

	// compute the root hash
	rootHash := rootNode.Hash()

	// verify root hash against SMH
	if !bytes.Equal(rootHash, response.SignedMapHead.RootHash) {
		return fmt.Errorf("computed root hash does not match the SMH")
	}

	// verification succeeded, return certificates
	return nil
}
