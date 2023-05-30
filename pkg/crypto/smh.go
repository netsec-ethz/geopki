package crypto

import (
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/binary"
	"geopki/pkg/comm"
)

// the map head
type MapHead struct {
	// the root hash of the tree
	RootHash SHA256Hash

	// the unix timestamp in seconds when this SMH / version was created
	Timestamp uint64
}

type SignedMapHead struct {
	// the map head the signature is computed over
	MapHead

	// the signature over the MH
	Signature []byte
}

// returns the bytes to be signed
func (smh *MapHead) TBSBytes() []byte {
	bytes := smh.RootHash

	timestampBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(timestampBytes, smh.Timestamp)

	bytes = append(bytes, timestampBytes...)

	return bytes
}

func (smh *SignedMapHead) Sign(privateKey *ecdsa.PrivateKey) error {
	signature, err := ecdsa.SignASN1(rand.Reader, privateKey, smh.TBSBytes())

	if err != nil {
		return err
	}

	smh.Signature = signature

	return nil
}

func (smh *SignedMapHead) Verify(publicKey *ecdsa.PublicKey) bool {
	return ecdsa.VerifyASN1(publicKey, smh.TBSBytes(), smh.Signature)
}

func (smh *SignedMapHead) Proto() *comm.SignedMapHead {
	return &comm.SignedMapHead{
		RootHash:  smh.RootHash,
		Timestamp: smh.Timestamp,
		Signature: smh.Signature,
	}
}

func NewSMHFromCommSMH(smh *comm.SignedMapHead) *SignedMapHead {
	return &SignedMapHead{
		MapHead: MapHead{
			RootHash:  smh.RootHash,
			Timestamp: smh.Timestamp,
		},
		Signature: smh.Signature,
	}
}
