package crypto

import (
	"crypto/sha256"
	"fmt"
	"log"

	"geopki/pkg/bitstring"
)

type Node struct {
	// the raw bit string associated with this node
	bitstring.RawBitStringPair

	xyLeftChild, xyRightChild, zLeftChild, zRightChild *Node

	// the hashes of all children
	// can be nil when sent as a response or when it is equal to the default hash
	xyLeftChildHash, xyRightChildHash, zLeftChildHash, zRightChildHash SHA256Hash

	CertificateHashes []SHA256Hash
}

func NewDBNode(
	XYBitString uint64,
	XYBitStringLen uint8,

	ZBitString uint16,
	ZBitStringLen uint8,

	xyLeftChildHash, xyRightChildHash, zLeftChildHash, zRightChildHash SHA256Hash,
	certificateHashes []SHA256Hash,
) *Node {

	return &Node{
		RawBitStringPair: bitstring.RawBitStringPair{
			RawXYBitString: bitstring.RawXYBitString{
				XYBitString:    XYBitString,
				XYBitStringLen: XYBitStringLen,
			},
			RawZBitString: bitstring.RawZBitString{
				ZBitString:    ZBitString,
				ZBitStringLen: ZBitStringLen,
			},
		},

		xyLeftChildHash:  xyLeftChildHash,
		xyRightChildHash: xyRightChildHash,
		zLeftChildHash:   zLeftChildHash,
		zRightChildHash:  zRightChildHash,

		CertificateHashes: certificateHashes,
	}
}

func NewTreeNode(
	xyBitString uint64,
	xyBitStringLen uint8,

	zBitString uint16,
	zBitStringLen uint8,

	xyLeftChildHash, xyRightChildHash, zLeftChildHash, zRightChildHash []byte,
	certificateHashes []SHA256Hash,
) *Node {

	return &Node{
		RawBitStringPair: bitstring.RawBitStringPair{
			RawXYBitString: bitstring.RawXYBitString{
				XYBitString:    xyBitString,
				XYBitStringLen: xyBitStringLen,
			},
			RawZBitString: bitstring.RawZBitString{
				ZBitString:    zBitString,
				ZBitStringLen: zBitStringLen,
			},
		},

		xyLeftChildHash:   xyLeftChildHash,
		xyRightChildHash:  xyRightChildHash,
		zLeftChildHash:    zLeftChildHash,
		zRightChildHash:   zRightChildHash,
		CertificateHashes: certificateHashes,
	}
}

// returns XYBitString and ZBitString as a pair struct
func (node *Node) Pair() bitstring.RawBitStringPair {
	return node.RawBitStringPair
}

// returns the left xy child hash. if 'useDefault' is set, returns SHA256(0) if nil
func (node *Node) XYLeftChildHash(useDefault bool) SHA256Hash {
	if node.xyLeftChild != nil {
		return node.xyLeftChild.Hash()
	}

	if (!useDefault) || node.xyLeftChildHash != nil {
		return node.xyLeftChildHash
	}

	return DEFAULT_HASH
}

// sets the xy left left hash
func (node *Node) SetXYLeftChildHash(xyLeftChildHash SHA256Hash) error {
	if node.xyLeftChild != nil {
		return fmt.Errorf("tried setting xyLeftChild on node with non-nil xyLeftChild")
	}

	node.xyLeftChildHash = xyLeftChildHash

	return nil
}

// sets the xy left left node
func (node *Node) SetXYLeftChild(xyLeftChild *Node) error {
	// can fail for non 2D nodes
	child, err := node.RawBitStringPair.XYLeftChildPair()

	if err != nil {
		return err
	}

	if !child.Equals(xyLeftChild.RawBitStringPair) {
		return fmt.Errorf("tried setting invalid xy left child")
	}

	if node.xyLeftChildHash != nil {
		return fmt.Errorf("tried setting xyLeftChildHash on node with non-nil xyLeftChild hash")
	}

	node.xyLeftChild = xyLeftChild

	return nil
}

// sets 'xyLeftChild' and 'xyLeftChildHash' to nil
func (node *Node) ClearXYLeftChild() {
	node.xyLeftChild = nil
	node.xyLeftChildHash = nil
}

// returns the right xy child hash. if 'useDefault' is set, returns SHA256(0) if nil
func (node *Node) XYRightChildHash(useDefault bool) SHA256Hash {
	if node.xyRightChild != nil {
		return node.xyRightChild.Hash()
	}

	if (!useDefault) || node.xyRightChildHash != nil {
		return node.xyRightChildHash
	}

	return DEFAULT_HASH
}

// sets the xy right child hash
func (node *Node) SetXYRightChildHash(xyRightChildHash SHA256Hash) error {
	if node.xyRightChild != nil {
		return fmt.Errorf("tried setting xyRightChildHash on node with non-nil xyRightChild")
	}

	node.xyRightChildHash = xyRightChildHash

	return nil
}

// sets the xy right child node
func (node *Node) SetXYRightChild(xyRightChild *Node) error {
	// can fail for non 2D nodes
	child, err := node.RawBitStringPair.XYRightChildPair()

	if err != nil {
		return err
	}

	if !child.Equals(xyRightChild.RawBitStringPair) {
		return fmt.Errorf("tried setting invalid xy right child")
	}

	if node.xyRightChildHash != nil {
		return fmt.Errorf("tried setting xyRightChild on node with non-nil xyRightChild hash")
	}

	node.xyRightChild = xyRightChild

	return nil
}

// sets 'xyRightChild' and 'xyRightChildHash' to nil
func (node *Node) ClearXYRightChild() {
	node.xyRightChild = nil
	node.xyRightChildHash = nil
}

// returns the left z child hash. if 'useDefault' is set, returns SHA256(0) if nil
func (node *Node) ZLeftChildHash(useDefault bool) SHA256Hash {
	if node.zLeftChild != nil {
		return node.zLeftChild.Hash()
	}

	if (!useDefault) || node.zLeftChildHash != nil {
		return node.zLeftChildHash
	}

	return DEFAULT_HASH
}

// sets the z left left hash
func (node *Node) SetZLeftChildHash(zLeftChildHash SHA256Hash) error {
	if node.zLeftChild != nil {
		return fmt.Errorf("tried setting zLeftChild on node with non-nil zLeftChild")
	}

	node.zLeftChildHash = zLeftChildHash

	return nil
}

// sets the z left left node
func (node *Node) SetZLeftChild(zLeftChild *Node) error {
	if !node.RawBitStringPair.ZLeftChildPair().Equals(zLeftChild.RawBitStringPair) {
		return fmt.Errorf("tried setting invalid z left child")
	}

	if node.zLeftChildHash != nil {
		return fmt.Errorf("tried setting zLeftChild on node with non-nil zLeftChild hash")
	}

	node.zLeftChild = zLeftChild

	return nil
}

// sets 'zLeftChild' and 'zLeftChildHash' to nil
func (node *Node) ClearZLeftChild() {
	node.zLeftChild = nil
	node.zLeftChildHash = nil
}

// returns the right z child hash. if 'useDefault' is set, returns SHA256(0) if nil
func (node *Node) ZRightChildHash(useDefault bool) SHA256Hash {
	if node.zRightChild != nil {
		return node.zRightChild.Hash()
	}

	if (!useDefault) || node.zRightChildHash != nil {
		return node.zRightChildHash
	}

	return DEFAULT_HASH
}

// sets the xy right child hash
func (node *Node) SetZRightChildHash(zRightChildHash SHA256Hash) error {
	if node.zRightChild != nil {
		return fmt.Errorf("tried setting zRightChildHash on node with non-nil zRightChild")
	}

	node.zRightChildHash = zRightChildHash

	return nil
}

// sets the xy right child node
func (node *Node) SetZRightChild(zRightChild *Node) error {
	if !node.RawBitStringPair.ZRightChildPair().Equals(zRightChild.RawBitStringPair) {
		return fmt.Errorf("tried setting invalid z right child")
	}

	if node.zRightChildHash != nil {
		return fmt.Errorf("tried setting zRightChild on node with non-nil zRightChild hash")
	}

	node.zRightChild = zRightChild

	return nil
}

// sets 'xyRightChild' and 'xyRightChildHash' to nil
func (node *Node) ClearZRightChild() {
	node.zRightChild = nil
	node.zRightChildHash = nil
}

func (node *Node) ConcatenatedCertificateHashes() []byte {
	bytes := []byte{}
	for _, certificateHash := range node.CertificateHashes {
		bytes = append(bytes, certificateHash...)
	}

	return bytes
}

// computes the hash of the node
func (node *Node) Hash() SHA256Hash {
	if node.XYBitStringLen > bitstring.XY_BITS || node.ZBitStringLen > bitstring.Z_BITS {
		log.Fatalf("invalid bit string pair with sizes (%d, %d)", node.XYBitStringLen, node.ZBitStringLen)
	} else if node.XYBitStringLen == bitstring.XY_BITS && node.ZBitStringLen == bitstring.Z_BITS {
		// hash of a leaf
		hash := sha256.Sum256(
			append(
				// prepend 0x00
				[]byte{0x00},
				node.ConcatenatedCertificateHashes()...,
			),
		)

		return hash[:]
	}

	// prepend 0x01
	bytes := []byte{0x01}
	// append all child hashes or DEFAULT_HASH if one of them is nil
	bytes = append(bytes, node.XYLeftChildHash(true)...)
	bytes = append(bytes, node.XYRightChildHash(true)...)
	bytes = append(bytes, node.ZLeftChildHash(true)...)
	bytes = append(bytes, node.ZRightChildHash(true)...)

	// hash of an intermediate node
	if len(node.CertificateHashes) == 0 {
		hash := sha256.Sum256(bytes)
		return hash[:]
	} else {
		certificateHash := sha256.Sum256(node.ConcatenatedCertificateHashes())

		hash := sha256.Sum256(
			append(
				bytes,
				certificateHash[:]...,
			),
		)

		return hash[:]
	}
}

// counts the number of nodes in the subtree rooted at this node
// can overflow if there are more than 2^31 - 1 nodes but
// there will be other problems too
func (node *Node) CountNodes() int {
	c := 1

	if node.xyLeftChild != nil {
		c += node.xyLeftChild.CountNodes()
	}

	if node.xyRightChild != nil {
		c += node.xyRightChild.CountNodes()
	}

	if node.zLeftChild != nil {
		c += node.zLeftChild.CountNodes()
	}

	if node.zRightChild != nil {
		c += node.zRightChild.CountNodes()
	}

	return c
}

// walks a given path (consisting of '0' and '1') until either
// the path is exhausted or a given child is not found
// returns the found node and the remaining path
func (node *Node) Walk(
	xyPath, zPath []rune,
) (*Node, []rune, []rune) {
	if len(xyPath) == 0 {
		if len(zPath) == 0 {
			// exhausted path
			return node, xyPath, zPath
		}

		//xyPath is exhausted, walk zPath
		if zPath[0] == '0' {
			if node.zLeftChild == nil {
				// cannot continue path further, return this node
				return node, xyPath, zPath
			} else {
				return node.zLeftChild.Walk(xyPath, zPath[1:])
			}
		} else {
			if node.zRightChild == nil {
				// cannot continue path further, return this node
				return node, xyPath, zPath
			} else {
				return node.zRightChild.Walk(xyPath, zPath[1:])
			}
		}
	} else {
		if xyPath[0] == '0' {
			if node.xyLeftChild == nil {
				// cannot continue path further, return this node
				return node, xyPath, zPath
			} else {
				return node.xyLeftChild.Walk(xyPath[1:], zPath)
			}
		} else {
			if node.xyRightChild == nil {
				// cannot continue path further, return this node
				return node, xyPath, zPath
			} else {
				return node.xyRightChild.Walk(xyPath[1:], zPath)
			}
		}
	}
}

// checks if the zSubtree is complete with respect to
// a min and max altitude
func (node *Node) IsZComplete(
	altitudeMin, altitudeMax int16,
) bool {
	isComplete := true

	leftChild := node.RawZBitString.LeftChild().BitString()
	leftChildMin, leftChildMax := int16(leftChild.ZMin), int16(leftChild.ZMax())

	rightChild := node.RawZBitString.LeftChild().BitString()
	rightChildMin, rightChildMax := int16(rightChild.ZMin), int16(rightChild.ZMax())

	// check for intersection of the 1 dimensional lines
	// line1 = (a1, b1), line2 = (a2, b2)
	// they do not intersect if one of them starts after the other ends, i.e.
	// b1 < a2 || b2 < a1 ----> not(b1 < a2 || b2 < a1) <=> b1 >= a2 && b2 >= a1
	// from perspective of line1: <=> a1 <= b2 && b1 >= a2
	// from perspective of line2: <=> a2 <= b1 && b2 >= a1
	if altitudeMin <= leftChildMax && altitudeMax >= leftChildMin {
		// intersection with left child
		if node.zLeftChild == nil {
			// intersects but server did not include the node in the response
			// only valid if the subtree is empty
			isComplete = isComplete && (node.zLeftChildHash == nil)
		} else {
			isComplete = node.zLeftChild.IsZComplete(altitudeMin, altitudeMax)
		}
	}

	if altitudeMin <= rightChildMax && altitudeMax >= rightChildMin {
		// intersection with right child
		if node.zRightChild == nil {
			// intersects but server did not include the node in the response
			// only valid if the subtree is empty
			isComplete = isComplete && (node.zRightChild == nil)
		} else {
			isComplete = node.zRightChild.IsZComplete(altitudeMin, altitudeMax)
		}
	}

	return isComplete
}

// checks if all subtrees contained in the subtree
// rooted at this node are complete
func (node *Node) IsComplete(
	minAltitude, maxAltitude int16,
) bool {
	isComplete := true

	if node.xyLeftChild == nil {
		// the server did not send this node which we requested,
		// the only valid reason is if that subtree is empty = has a default hash value
		isComplete = isComplete && (node.xyLeftChildHash == nil)
	} else {
		// recurse
		isComplete = isComplete && node.xyLeftChild.IsComplete(minAltitude, maxAltitude)
	}

	if node.xyRightChild == nil {
		// the server did not send this node which we requested,
		// the only valid reason is if that subtree is empty = has a default hash value
		isComplete = isComplete && (node.xyRightChildHash == nil)
	} else {
		// recurse
		isComplete = isComplete && node.xyRightChild.IsComplete(minAltitude, maxAltitude)
	}

	if node.zLeftChild == nil {
		// the server did not send this node which we requested,
		// the only valid reason is if that subtree is empty = has a default hash value
		isComplete = isComplete && (node.zLeftChildHash == nil)
	} else {
		// check z subtree for completeness
		isComplete = isComplete && node.zLeftChild.IsZComplete(minAltitude, maxAltitude)
	}

	if node.zRightChild == nil {
		// the server did not send this node which we requested,
		// the only valid reason is if that subtree is empty = has a default hash value
		isComplete = isComplete && (node.zRightChildHash == nil)
	} else {
		// check z subtree for completeness
		isComplete = isComplete && node.zRightChild.IsZComplete(minAltitude, maxAltitude)
	}

	return isComplete
}

// checks if the zSubtrees of all nodes along the path are complete
// with respect to minAltitude and maxAltitude
// AND that the full subtree rooted at the node at the end of the path
// has a complete zSubtrees with respect to minAltitude and maxAltitude
func (node *Node) PathIsComplete(
	xyPath []rune,
	minAltitude, maxAltitude int16,
) bool {

	if len(xyPath) == 0 {
		// exhausted path, this subtree must be fully complete
		return node.IsComplete(minAltitude, maxAltitude)
	} else {
		// check z subtree of node for completeness
		if !node.IsZComplete(minAltitude, maxAltitude) {
			// certainly not complete since one of the z trees along the path is not
			return false
		}
		// complete so far, continue checks

		if xyPath[0] == '0' {
			if node.xyLeftChild == nil {
				// the server did not send this node which we requested,
				// the only valid reason is if that subtree is empty = has a default hash value
				return node.xyLeftChildHash == nil
			} else {
				return node.xyLeftChild.PathIsComplete(xyPath[1:], minAltitude, maxAltitude)
			}
		} else {
			if node.xyRightChild == nil {
				// the server did not send this node which we requested,
				// the only valid reason is if that subtree is empty
				return node.xyRightChildHash == nil
			} else {
				return node.xyRightChild.PathIsComplete(xyPath[1:], minAltitude, maxAltitude)
			}
		}
	}

}
