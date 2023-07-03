package server

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"fmt"
	"geopki/pkg/crypto"
	"geopki/pkg/database"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"
)

// creates a new SMH by querying the db and signing the root hash
func CreateNewSMH(
	tx pgx.Tx,
	privateKey *ecdsa.PrivateKey,
	ctx context.Context,
) (
	*crypto.SignedMapHead,
	error,
) {

	rootHash, err := database.QueryRootHash(tx, ctx)
	if err != nil {
		// root node is not in DB -> is sparse / tree is empty
		rootHash = crypto.DEFAULT_HASH
	}

	// create new SMH *WITHOUT* signature
	smh := &crypto.SignedMapHead{
		MapHead: crypto.MapHead{
			RootHash:  rootHash,
			Timestamp: uint64(time.Now().UnixNano()),
			// TODO: set the set of covered log servers for instance by storing that in the db and retrieving it here
			CoveredCTLogServers: []crypto.CTLogServer{},
		},
	}

	// sign SMH
	err = smh.Sign(privateKey)
	if err != nil {
		return nil, err
	}

	return smh, nil
}

// creates a new sch and updates the cached values in env, the caller must ensure a lock is held
func (env *EndpointHandlerEnv) updateSCH(
	smh *crypto.SignedMapHead,
	ctx context.Context,
) (
	*crypto.SignedConsistencyHead,
	error,
) {
	// insert it into the consistency tree
	sch, err := env.ConsistencyClient.AppendSignedMapHead(ctx, smh)
	if err != nil {
		return nil, fmt.Errorf("failed updating the consistency tree: %v", err)
	}

	proof, err := env.ConsistencyClient.ProveSignedMapHeadInclusion(ctx, sch.Size, smh)
	if err != nil {
		return nil, fmt.Errorf("obtaining a proof of inclusion for consistency tree failed: %v", err)
	}

	marshaledProof, err := proto.Marshal(proof)
	if err != nil {
		return nil, fmt.Errorf("failed marshaling inclusion proof: %v", err)
	}

	// update cached smh and sch, acquire lock to ensure all reads are consistent
	env.CurrentSignedMapHead = smh
	env.CurrentSignedConsistencyHead = sch
	env.SchInclusionProof = marshaledProof

	return sch, nil
}

// ensures the GET parameter 'key' is set to the correct value
func (env *EndpointHandlerEnv) receivedValidInsertionKey(c *gin.Context) bool {
	key := c.DefaultQuery("key", "???")
	keyHash := sha256.Sum256([]byte(key))

	// compare hashes, avoids timing side channel since the strings are of the same length
	if !bytes.Equal(keyHash[:], env.CertificateInsertionKeyHash) {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "invalid key",
		})
		return false
	}

	return true
}
