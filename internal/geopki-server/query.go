package server

import (
	"encoding/base64"
	"fmt"
	"os"

	"geopki/pkg/comm"
	"geopki/pkg/database"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/valyala/fasthttp"
	"google.golang.org/protobuf/proto"
)

// handler for the /query endpoint
func (env *EndpointHandlerEnv) postQuery(ctx *fasthttp.RequestCtx) {

	// acquire read lock on cache for the duration of the query to guarantee
	// the cached data is consistent with the data retrieved from the db
	env.SharedDataLock.RLock()
	defer env.SharedDataLock.RUnlock()

	includeCertificates := ctx.QueryArgs().Has("c")

	// read request body
	requestBitStringPairs, minAltitude, maxAltitude, err := comm.ParseQuery(ctx.Request.Body())
	if err != nil {
		errorHandler(ctx, fasthttp.StatusBadRequest, err.Error())
		return
	}

	sqlQuery := database.BuildNodeQuery(requestBitStringPairs, minAltitude, maxAltitude)

	rows, err := env.DbPool.Query(
		ctx,
		sqlQuery,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "node query failed: %v\n", err)
		errorHandler(ctx, fasthttp.StatusInternalServerError, "database query failed")
		return
	}

	defer rows.Close()

	// allocate slice with capacity of 'len(bit_strings)' and let go handle slice growth
	nodes, rootHash, err := database.RowsToNodesAndRootHash(rows, len(requestBitStringPairs))
	if err != nil {
		fmt.Fprintf(os.Stderr, "scanning node rows failed: %v\n", err)
		errorHandler(ctx, fasthttp.StatusInternalServerError, "scanning rows failed")
		return
	}

	if rootHash == nil {
		fmt.Fprintf(os.Stderr, "integrity check failed, root node was not returned by the query")
		errorHandler(ctx, fasthttp.StatusInternalServerError, "integrity check failed, root node was not returned by the query")
		return
	}

	var certificates [][]byte
	if includeCertificates {

		// create set of all certificate
		certificateStringHashes := mapset.NewThreadUnsafeSet[string]()
		for _, n := range nodes {
			for _, certificateHash := range n.CertificateHashes {
				certificateStringHashes.Add(base64.RawURLEncoding.EncodeToString(certificateHash))
			}
		}

		// no need to continue if no certificates are requested
		if certificateStringHashes.Cardinality() > 0 {
			sqlQuery, err := database.BuildCertificateQuery(certificateStringHashes)
			if err != nil {
				fmt.Fprintf(os.Stderr, "building certificate query failed: %v\n", err)
				errorHandler(ctx, fasthttp.StatusInternalServerError, "building database query failed")
				return
			}

			rows, err := env.DbPool.Query(
				ctx,
				sqlQuery,
			)
			if err != nil {
				fmt.Fprintf(os.Stderr, "certificate query failed: %v\n", err)
				errorHandler(ctx, fasthttp.StatusInternalServerError, "database query failed")
				return
			}

			defer rows.Close()

			// allocate slice with capacity of 'certificateStringHashes.Cardinality()', let go handle slice growth
			certificates, err = database.RowsToCertificates(rows, certificateStringHashes.Cardinality())
			if err != nil {
				fmt.Fprintf(os.Stderr, "scanning certificate rows failed: %v\n", err)
				errorHandler(ctx, fasthttp.StatusInternalServerError, "scanning rows failed")
				return
			}
		}
	}

	sch := env.CurrentSignedConsistencyHead
	smh := env.CurrentSignedMapHead
	inclusionProof := env.SchInclusionProof

	response, err := proto.Marshal(&comm.Response{
		SignedConsistencyHead: sch,
		SignedMapHead:         smh,
		InclusionProof:        inclusionProof,
		Nodes:                 nodes,

		Certificates: certificates,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed marshaling response: %v\n", err)
		errorHandler(ctx, fasthttp.StatusInternalServerError, "failed marshaling response")
		return
	}

	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBody(response)
}

// handler for the /certificates endpoint
func (env *EndpointHandlerEnv) getCertificates(ctx *fasthttp.RequestCtx) {
	certificateStringHashes := ctx.QueryArgs().PeekMulti("hash")
	if len(certificateStringHashes) == 0 {
		errorHandler(ctx, fasthttp.StatusBadRequest, "failed marshaling response")
		return
	}

	certificateStringHashMap := mapset.NewThreadUnsafeSet[string]()
	for _, certificateStringHash := range certificateStringHashes {
		certificateStringHashMap.Add(string(certificateStringHash))
	}

	var certificates [][]byte
	sqlQuery, err := database.BuildCertificateQuery(
		certificateStringHashMap,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "building certificate query failed: %v\n", err)
		errorHandler(ctx, fasthttp.StatusBadRequest, "building database query failed")
		return
	}

	rows, err := env.DbPool.Query(
		ctx,
		sqlQuery,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "certificate query failed: %v\n", err)
		errorHandler(ctx, fasthttp.StatusBadRequest, "database query failed")
		return
	}

	defer rows.Close()

	// at least allocate a capacity of 'len(bit_strings)', then let
	// the go standard libary handle growth
	certificates, err = database.RowsToCertificates(rows, len(certificateStringHashes))
	if err != nil {
		fmt.Fprintf(os.Stderr, "scanning certificate rows failed: %v\n", err)
		errorHandler(ctx, fasthttp.StatusBadRequest, "scanning rows failed")
		return
	}

	response, err := proto.Marshal(&comm.Response{
		Certificates: certificates,
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "failed marshaling response: %v\n", err)
		errorHandler(ctx, fasthttp.StatusBadRequest, "failed marshaling response")
		return
	}

	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBody(response)
}
