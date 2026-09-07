package crypto

import (
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"

	"geopki/pkg/comm"

	"google.golang.org/protobuf/proto"
)

// type to indentify a ct log server version, i.e. a specific STH
type CTLogServer struct {
	// the id of the log
	LogId []byte

	// the latest version of the log that is covered
	SignedTreeHead []byte
}

// the map head
type MapHead struct {
	// the root hash of the tree
	RootHash SHA256Hash

	// the unix timestamp in seconds when this version was created
	Timestamp uint64

	// the set of covered ct log servers
	CoveredCTLogServers []CTLogServer
}

type SignedMapHead struct {
	// the map head the signature is computed over
	MapHead

	// the signature over the MH
	Signature []byte
}

// returns the bytes to be signed
func (smh *MapHead) TBSBytes() []byte {
	tbsBytes := smh.RootHash[:]

	timestampBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(timestampBytes, smh.Timestamp)
	tbsBytes = append(tbsBytes, timestampBytes...)

	for _, coveredCTLogServers := range smh.CoveredCTLogServers {
		tbsBytes = append(tbsBytes, coveredCTLogServers.LogId...)
		tbsBytes = append(tbsBytes, coveredCTLogServers.SignedTreeHead...)
	}

	return tbsBytes
}

// signs the SMH
func (smh *SignedMapHead) Sign(privateKey *ecdsa.PrivateKey) error {
	signature, err := ecdsa.SignASN1(rand.Reader, privateKey, smh.TBSBytes())

	if err != nil {
		return err
	}

	smh.Signature = signature

	return nil
}

// verifies the signature on the SMH using a public key
func (smh *SignedMapHead) Verify(publicKey *ecdsa.PublicKey) bool {
	return ecdsa.VerifyASN1(publicKey, smh.TBSBytes(), smh.Signature)
}

// serializes the SMH
func (smh *SignedMapHead) Proto() *comm.SignedMapHead {
	// create list of covered log servers
	coveredCTLogServers := make([]*comm.CTLogServer, len(smh.CoveredCTLogServers))
	for i, coveredCTLogServer := range smh.CoveredCTLogServers {
		coveredCTLogServers[i] = &comm.CTLogServer{
			LogId:          coveredCTLogServer.LogId,
			SignedTreeHead: coveredCTLogServer.SignedTreeHead,
		}
	}

	return &comm.SignedMapHead{
		RootHash:            smh.RootHash[:],
		Timestamp:           smh.Timestamp,
		Signature:           smh.Signature,
		CoveredCTLogServers: coveredCTLogServers,
	}
}

// serializes the smh (including the signature)
func (smh *SignedMapHead) Marshal() ([]byte, error) {
	return proto.Marshal(smh.Proto())
}

// returns a string representation for debugging
func (smh *MapHead) String() string {
	s := ""
	s += fmt.Sprintf("%s at %d\n", base64.StdEncoding.EncodeToString(smh.RootHash[:]), smh.Timestamp)

	for _, coveredCTLogServers := range smh.CoveredCTLogServers {
		s += fmt.Sprintf("    %s, %s\n", base64.StdEncoding.EncodeToString(coveredCTLogServers.LogId), base64.StdEncoding.EncodeToString(coveredCTLogServers.SignedTreeHead))
	}

	return s
}

// creates a new SMH instance from received comm.SignedMapHead instances
func NewSMHFromCommSMH(smh *comm.SignedMapHead) *SignedMapHead {

	// create list of covered log servers
	coveredCTLogServers := make([]CTLogServer, len(smh.GetCoveredCTLogServers()))
	for i, coveredCTLogServer := range smh.GetCoveredCTLogServers() {
		coveredCTLogServers[i] = CTLogServer{
			LogId:          coveredCTLogServer.GetLogId(),
			SignedTreeHead: coveredCTLogServer.GetSignedTreeHead(),
		}
	}

	return &SignedMapHead{
		MapHead: MapHead{
			RootHash:            BytesToHash(smh.RootHash),
			Timestamp:           smh.Timestamp,
			CoveredCTLogServers: coveredCTLogServers,
		},
		Signature: smh.Signature,
	}
}

// creates a new SMH instanced based from received bytes
func UnmarshalSignedMapHead(data []byte) (*SignedMapHead, error) {
	smh := new(comm.SignedMapHead)

	err := proto.Unmarshal(data, smh)
	if err != nil {
		return nil, err
	}

	return NewSMHFromCommSMH(smh), nil
}
