package crypto

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"geopki/pkg/comm"
	"time"

	"github.com/google/trillian"
	trillianTypes "github.com/google/trillian/types"
	"github.com/transparency-dev/merkle/rfc6962"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
)

// a head of the consistency tree
type ConsistencyHead struct {
	// the root hash of the tree
	RootHash SHA256Hash

	// the unix timestamp in nanoseconds when this version was created
	Timestamp uint64

	// the number of entries in the log this version was created
	Size uint64
}

// a signed head of the consistency tree
type SignedConsistencyHead struct {
	// the map head the signature is computed over
	ConsistencyHead

	// the signature over the MH
	Signature []byte
}

// returns the bytes to be signed
func (smh *ConsistencyHead) TBSBytes() []byte {
	bytes := smh.RootHash

	timestampBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(timestampBytes, smh.Timestamp)

	bytes = append(bytes, timestampBytes...)

	sizeBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(sizeBytes, smh.Size)

	bytes = append(bytes, sizeBytes...)

	return bytes
}

// signs the SCH, overrides the Signature field
func (smh *SignedConsistencyHead) Sign(privateKey *ecdsa.PrivateKey) error {
	signature, err := ecdsa.SignASN1(rand.Reader, privateKey, smh.TBSBytes())

	if err != nil {
		return err
	}

	smh.Signature = signature

	return nil
}

// verifies the Singature field against the TBSBytes
func (sch *SignedConsistencyHead) Verify(publicKey *ecdsa.PublicKey) bool {
	return ecdsa.VerifyASN1(publicKey, sch.TBSBytes(), sch.Signature)
}

// creates a protobuf message based on the SCH
func (smh *SignedConsistencyHead) Proto() *comm.SignedConsistencyHead {
	return &comm.SignedConsistencyHead{
		RootHash:  smh.RootHash,
		Timestamp: smh.Timestamp,
		Size:      smh.Size,
		Signature: smh.Signature,
	}
}

// serializes the smh (including the signature) to protobuf bytes
func (sch *SignedConsistencyHead) Marshal() ([]byte, error) {
	return proto.Marshal(sch.Proto())
}

// creates a signed consistency head instance from a protocol buffer instance
func NewSCHFromCommSCH(sch *comm.SignedConsistencyHead) *SignedConsistencyHead {
	return &SignedConsistencyHead{
		ConsistencyHead: ConsistencyHead{
			RootHash:  sch.RootHash,
			Timestamp: sch.Timestamp,
			Size:      sch.Size,
		},
		Signature: sch.Signature,
	}
}

// unmarshals a signed consistency head
func UnmarshalSignedConsistencyHead(data []byte) (*SignedConsistencyHead, error) {
	sch := new(comm.SignedConsistencyHead)

	err := proto.Unmarshal(data, sch)
	if err != nil {
		return nil, err
	}

	return NewSCHFromCommSCH(sch), nil
}

func SignConsistencyHead(consistencyHead *ConsistencyHead, privateKey *ecdsa.PrivateKey) (*SignedConsistencyHead, error) {
	sch := &SignedConsistencyHead{
		ConsistencyHead: *consistencyHead,
	}

	err := sch.Sign(privateKey)
	if err != nil {
		return nil, err
	}

	return sch, nil
}

// Based on https://github.com/google/trillian-examples/blob/master/helloworld/client.go
// licensed under Apache v2 (https://www.apache.org/licenses/LICENSE-2.0).
// The referenced code was modified

// a trillian 'personality'
type ConsistencyTreeClient struct {
	// client instance that allows accessing trillian
	logClient trillian.TrillianLogClient
	// identifies the tree, trillian is multi-tenant
	logId int64
	// the private key used to sign statements
	privateKey *ecdsa.PrivateKey

	// the maximum merge delay in seconds
	maximumMergeDelay uint8
}

// creates a new consistency tree client which is a specialized trillian personality
func NewConsistencyTreeClient(logAddr string, logId int64, privateKey *ecdsa.PrivateKey, maximumMergeDelay uint8) (*ConsistencyTreeClient, error) {
	if logId <= 0 {
		return nil, fmt.Errorf("invalid tree id %d", logId)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// connection to the local server, no transport security required
	conn, err := grpc.DialContext(ctx, logAddr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %v", logAddr, err)
	}

	return &ConsistencyTreeClient{
		logClient:         trillian.NewTrillianLogClient(conn),
		logId:             logId,
		privateKey:        privateKey,
		maximumMergeDelay: maximumMergeDelay,
	}, nil
}

// formLeaf creates a trillian log leaf from a signed map head
func (p *ConsistencyTreeClient) formLeaf(smh *SignedMapHead) (*trillian.LogLeaf, error) {
	marshaledSMH, err := smh.Marshal()
	if err != nil {
		return nil, err
	}

	leafHash := rfc6962.DefaultHasher.HashLeaf(marshaledSMH)

	return &trillian.LogLeaf{
		LeafValue:      marshaledSMH,
		MerkleLeafHash: leafHash,
	}, nil
}

// getConsistencyHead fetches the latest trillian root and creates a checkpoint from it
func (p *ConsistencyTreeClient) getConsistencyHead(ctx context.Context) (*ConsistencyHead, error) {
	req := trillian.GetLatestSignedLogRootRequest{LogId: p.logId}

	resp, err := p.logClient.GetLatestSignedLogRoot(ctx, &req)
	if err != nil {
		return nil, err
	}

	// Unpack the response and convert it to the local Checkpoint
	// representation.
	root := resp.GetSignedLogRoot()

	var logRoot trillianTypes.LogRootV1
	if err := logRoot.UnmarshalBinary(root.LogRoot); err != nil {
		return nil, err
	}

	return &ConsistencyHead{
		RootHash:  logRoot.RootHash,
		Timestamp: logRoot.TimestampNanos,
		Size:      logRoot.TreeSize,
	}, nil
}

func (p *ConsistencyTreeClient) InitializeLog(ctx context.Context) error {
	_, err := p.logClient.InitLog(ctx, &trillian.InitLogRequest{
		LogId: p.logId,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize log: %v", err)
	}

	return nil
}

// Gets the latest consistency head
func (p *ConsistencyTreeClient) LatestSignedConsistencyHead(ctx context.Context) (*SignedConsistencyHead, error) {
	consistencyHead, err := p.getConsistencyHead(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch consistency head: %v", err)
	}

	return SignConsistencyHead(consistencyHead, p.privateKey)
}

// Gets the latest map head, can be cached
func (p *ConsistencyTreeClient) LatestSignedMapHead(ctx context.Context) (*SignedMapHead, error) {
	consistencyHead, err := p.getConsistencyHead(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch consistency head: %v", err)
	}

	response, err := p.logClient.GetLeavesByRange(ctx, &trillian.GetLeavesByRangeRequest{
		LogId:      p.logId,
		StartIndex: int64(consistencyHead.Size) - 1,
		Count:      1,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch latest leaf: %v", err)
	}

	leaves := response.GetLeaves()
	if len(leaves) != 1 {
		return nil, fmt.Errorf("trillian returned %d leaves instead of 1", len(leaves))
	}

	latestLeaf := leaves[0]

	smh, err := UnmarshalSignedMapHead(latestLeaf.LeafValue)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal latest leaf: %v", err)
	}

	return smh, err
}

// Appends a signed map head to the log and waits to return the new SCH
// if the smh is already in the tree, the current SCH is returned
func (p *ConsistencyTreeClient) AppendSignedMapHead(ctx context.Context, smh *SignedMapHead) (*SignedConsistencyHead, error) {
	leaf, err := p.formLeaf(smh)
	if err != nil {
		return nil, err
	}

	// Get the latest consistencyHead
	consistencyHead, err := p.getConsistencyHead(ctx)
	if err != nil {
		return nil, err
	}

	// and check if the leaf is already present
	response, err := p.logClient.GetInclusionProofByHash(ctx, &trillian.GetInclusionProofByHashRequest{
		LogId:    p.logId,
		LeafHash: leaf.MerkleLeafHash,
		TreeSize: int64(consistencyHead.Size),
	})

	if response.GetProof() != nil && err == nil {
		return SignConsistencyHead(consistencyHead, p.privateKey)
	}

	// if not, add it
	request := trillian.QueueLeafRequest{
		LogId: p.logId,
		Leaf:  leaf,
	}
	if _, err := p.logClient.QueueLeaf(ctx, &request); err != nil {
		return nil, err
	}

	// Now fetch the new checkpoint, keep going until it is there and
	// return an error if it is not there after a certain amount of time
	for start := time.Now(); time.Since(start) < time.Duration(p.maximumMergeDelay)*time.Second; {
		consistencyHeadNew, err := p.getConsistencyHead(ctx)
		if err != nil {
			return nil, err
		}

		// check if the smh is in the tree by requesting a proof
		response, err := p.logClient.GetInclusionProofByHash(ctx, &trillian.GetInclusionProofByHashRequest{
			LogId:    p.logId,
			LeafHash: leaf.MerkleLeafHash,
			TreeSize: int64(consistencyHeadNew.Size),
		})

		if response.GetProof() != nil && err == nil {
			return SignConsistencyHead(consistencyHeadNew, p.privateKey)
		}
	}

	return nil, fmt.Errorf("did not get an updated checkpoint")
}

// returns an inclusion proof for a given tree size and smh hash
func (p *ConsistencyTreeClient) ProveSignedMapHeadHashInclusion(ctx context.Context, treeSize uint64, smhHash []byte) (*trillian.Proof, error) {

	// Form the request according to the trillian API
	request := trillian.GetInclusionProofByHashRequest{
		LogId:    p.logId,
		LeafHash: smhHash,
		TreeSize: int64(treeSize),
	}

	// Process the response
	response, err := p.logClient.GetInclusionProofByHash(ctx, &request)
	if err != nil {
		return nil, err
	}

	return response.GetProof()[0], nil
}

// returns an inclusion proof for a given tree size and smh
func (p *ConsistencyTreeClient) ProveSignedMapHeadInclusion(ctx context.Context, treeSize uint64, smh *SignedMapHead) (*trillian.Proof, error) {
	// Form the leaf from the entry.
	leaf, err := p.formLeaf(smh)
	if err != nil {
		return nil, err
	}

	return p.ProveSignedMapHeadHashInclusion(ctx, treeSize, leaf.MerkleLeafHash)
}

// gets the latest checkpoint for the trillian log and proves its
// consistency with a provided one
func (p *ConsistencyTreeClient) ConsistencyProof(ctx context.Context, treeSize1, treeSize2 uint64) (*trillian.Proof, error) {

	// Now get a consistency proof if one is needed.
	request := trillian.GetConsistencyProofRequest{
		LogId:          p.logId,
		FirstTreeSize:  int64(treeSize1),
		SecondTreeSize: int64(treeSize2),
	}

	response, err := p.logClient.GetConsistencyProof(ctx, &request)
	if err != nil {
		return nil, err
	}

	proof := response.GetProof()
	if proof == nil {
		return nil, fmt.Errorf("received empty proof, invalid input tree sizes?")
	}

	return proof, nil
}

// gets all entries within the given index range
func (p *ConsistencyTreeClient) GetEntries(ctx context.Context, start, end uint64) (*trillian.GetLeavesByRangeResponse, error) {

	// Now get a consistency proof if one is needed.
	request := trillian.GetLeavesByRangeRequest{
		LogId:      p.logId,
		StartIndex: int64(start),
		Count:      int64(end - start + 1),
	}

	response, err := p.logClient.GetLeavesByRange(ctx, &request)
	if err != nil {
		return nil, err
	}

	// this value is not required by clients
	response.SignedLogRoot = nil

	for _, e := range response.Leaves {
		e.LeafIdentityHash = nil
		e.QueueTimestamp = nil
		e.IntegrateTimestamp = nil
	}

	return response, nil
}

func (p *ConsistencyTreeClient) GetEntryAndProof(ctx context.Context, treeSize, leafIndex uint64) (*trillian.GetEntryAndProofResponse, error) {

	// Now get a consistency proof if one is needed.
	request := trillian.GetEntryAndProofRequest{
		LogId:     p.logId,
		TreeSize:  int64(treeSize),
		LeafIndex: int64(leafIndex),
	}

	response, err := p.logClient.GetEntryAndProof(ctx, &request)
	if err != nil {
		return nil, err
	}

	// this value is not required by clients
	response.SignedLogRoot = nil
	response.Leaf.LeafIdentityHash = nil
	response.Leaf.QueueTimestamp = nil
	response.Leaf.IntegrateTimestamp = nil

	return response, nil
}
