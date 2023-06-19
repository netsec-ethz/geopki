package crypto

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math"

	"geopki/pkg/bitstring"
	"geopki/pkg/comm"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/google/trillian"
	"github.com/transparency-dev/merkle/proof"
	"github.com/transparency-dev/merkle/rfc6962"
	"google.golang.org/protobuf/proto"
)

// verifies a recieved response based on the query and the server's public key
// and returns the set of all certificate hashes as hex strings
// this function does not perform any consistency checks
func VerifyResponse(response *comm.Response, query *comm.Query, publicKey *ecdsa.PublicKey) (mapset.Set[string], error) {
	if response.GetSignedMapHead() == nil {
		return nil, fmt.Errorf("response does not contain a SMH")
	}

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

		node := NewNode(
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
	if !bytes.Equal(rootHash, smh.RootHash) {
		return nil, fmt.Errorf("computed root hash does not match the SMH")
	}

	// ensure all requested nodes have been returned, or, the omitted subtrees are empty
	for _, xyBitString := range query.XYBitStrings {

		// find node corresponding to z subtree root or the
		// smallest ancestor of it
		ok := rootNode.PathIsComplete([]rune(xyBitString.BitString().String()), query.MinAltitude, query.MaxAltitude)
		if !ok {
			return nil, fmt.Errorf("server did not include all nodes required by the query (%s, %d, %d)", xyBitString.BitString().String(), query.MinAltitude, query.MaxAltitude)
		}
	}

	// verify that all received certificate's hash is in one of the nodes

	// first compute the set of all certificate hashes
	// unfortunately in string form since []byte is not comparable
	certificateStringHashes := mapset.NewSet[string]()
	for _, node := range ns {
		for _, certificateHash := range node.CertificateHashes {
			certificateStringHashes.Add(base64.RawURLEncoding.EncodeToString(certificateHash))
		}
	}

	for _, certificate := range response.GetCertificates() {
		hash := sha256.Sum256(certificate)
		hashString := base64.RawURLEncoding.EncodeToString(hash[:])

		if !certificateStringHashes.Contains(hashString) {
			return nil, fmt.Errorf("certificate with hash '%s' is part of the response but is not contained in any node", hashString)
		}
	}

	// verification succeeded, return certificates
	return certificateStringHashes, nil
}

func EnsureConsistency(response *comm.Response, publicKey *ecdsa.PublicKey) error {
	if response.GetSignedConsistencyHead() == nil {
		return fmt.Errorf("response does not contain a SCH")
	}

	sch := NewSCHFromCommSCH(response.GetSignedConsistencyHead())
	if !sch.Verify(publicKey) {
		return fmt.Errorf("signature on the SCH is invalid")
	}

	if response.GetSignedMapHead() == nil {
		return fmt.Errorf("response does not contain a SMH")
	}

	smh := NewSMHFromCommSMH(response.GetSignedMapHead())
	marshaledSMH, err := smh.Marshal()
	if err != nil {
		return nil
	}

	leafHash := rfc6962.DefaultHasher.HashLeaf(marshaledSMH)

	pf := new(trillian.Proof)
	err = proto.Unmarshal(response.InclusionProof, pf)

	if err != nil {
		return fmt.Errorf("failed unmarshaling: %v", err)
	}

	// https://github.com/google/trillian/blob/master/client/log_verifier.go#L90
	return proof.VerifyInclusion(rfc6962.DefaultHasher, uint64(pf.LeafIndex), sch.Size, leafHash, pf.Hashes, sch.RootHash)
}
