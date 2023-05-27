package crypto

import (
	"bytes"
	"fmt"
	"math"

	"geopki/pkg/bitstring"
	"geopki/pkg/comm"
)

var DEFAULT_HASH = []byte("\x6e\x34\x0b\x9c\xff\xb3\x7a\x98\x9c\xa5\x44\xe6\xbb\x78\x0a\x2c\x78\x90\x1d\x3f\xb3\x37\x38\x76\x85\x11\xa3\x06\x17\xaf\xa0\x1d")

func VerifyResponse(response *comm.Response, expectedRootHash []byte) ([][]byte, error) {
	ns := response.GetNodes()
	nodes := make([]*Node, len(ns))

	bitStringMap := make(map[bitstring.RawBitStringPair]*Node)

	var rootNode *Node
	certificates := make([][]byte, 0)

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

		node := NewTreeNode(
			n.XYBitString,
			uint8(n.XYBitStringLen),
			uint16(n.ZBitString),
			uint8(n.ZBitStringLen),
			n.CertificateHashes,
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

		certificates = append(certificates, n.Certificates...)
	}

	if rootNode == nil {
		return nil, fmt.Errorf("response did not contain the root node")
	}

	// build the tree
	for i, n := range nodes {
		node := nodes[i]

		if n.neighborHash == nil {
			neighbor, ok := bitStringMap[node.NeighborPair()]

			if !ok {
				node.SetNeighbor(neighbor)
			}

			// else: response did not contain neighbor nor neighbor hash
			// -> must be default hash -> set nothing

		} else {
			node.SetNeighborHash(n.neighborHash)
		}

		if n.xyLeftChildHash == nil {
			childBitString, err := node.XYLeftChildPair()
			// if err == nil child does not exist -> default hash -> do nothing

			if err != nil {
				child, ok := bitStringMap[childBitString]

				if ok {
					node.SetXYLeftChild(child)
				}

				// else: response did not contain child nor child hash
				// -> must be default hash -> set nothing
			}

		} else {
			node.SetXYLeftChildHash(n.xyLeftChildHash)
		}

		if n.xyRightChildHash == nil {
			childBitString, err := node.XYRightChildPair()
			// if err == nil child does not exist -> default hash -> do nothing

			if err != nil {
				child, ok := bitStringMap[childBitString]

				if ok {
					node.SetXYRightChild(child)
				}

				// else: response did not contain child nor child hash
				// -> must be default hash -> set nothing
			}

		} else {
			node.SetXYRightChildHash(n.xyRightChildHash)
		}

		if n.zLeftChildHash == nil {
			child, ok := bitStringMap[node.ZLeftChildPair()]

			if ok {
				node.SetZLeftChild(child)
			}

			// else: response did not contain child nor child hash
			// -> must be default hash -> set nothing

		} else {
			node.SetZLeftChildHash(n.zLeftChildHash)
		}

		if n.zRightChildHash == nil {
			child, ok := bitStringMap[node.ZRightChildPair()]

			if ok {
				node.SetZRightChild(child)
			}

			// else: response did not contain child nor child hash
			// -> must be default hash -> set nothing

		} else {
			node.SetZRightChildHash(n.zRightChildHash)
		}
	}

	// ensure all nodes are in the tree now
	if len(ns) != rootNode.CountNodes() {
		return nil, fmt.Errorf("received invalid tree, cannot use all nodes in tree")
	}

	// compute the root hash
	rootHash := rootNode.Hash()

	// verify root hash against SMH
	println(rootHash)
	if !bytes.Equal(rootHash, expectedRootHash) {
		return nil, fmt.Errorf("computed root hash does not match the expected one")
	}

	// verification succeeded, return certificates
	return certificates, nil
}
