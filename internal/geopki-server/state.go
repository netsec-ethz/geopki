package server

import (
	"crypto/ecdsa"
	"sync"

	"geopki/pkg/crypto"

	"github.com/jackc/pgx/v5/pgxpool"
)

type EndpointHandlerEnv struct {
	DbPool *pgxpool.Pool
	// key used to sign cryptographic statements
	PrivateKey *ecdsa.PrivateKey
	// key for inserting certificates
	CertificateInsertionKeyHash []byte

	// client for accessing the consistency tree
	ConsistencyClient *crypto.ConsistencyTreeClient

	// lock for accessing cached data or state data, not required for values defined
	// above since they never change
	SharedDataLock sync.RWMutex
	// lock for updating the DB
	UpdateLock sync.Mutex

	// caches the most recent SMH value
	CurrentSignedMapHead *crypto.SignedMapHead
	// caches the most recent SCH value
	CurrentSignedConsistencyHead *crypto.SignedConsistencyHead
	// caches the inclusion proof for the latest SCH value
	SchInclusionProof []byte
}
