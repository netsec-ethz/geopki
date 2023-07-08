package server

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"geopki/pkg/comm"
	"geopki/pkg/database"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"
)

// handler for the /query endpoint
func (env *EndpointHandlerEnv) postQuery(c *gin.Context) {

	// acquire read lock on cache for the duration of the query to guarantee
	// the cached data is consistent with the data retrieved from the db
	env.SharedDataLock.RLock()
	defer env.SharedDataLock.RUnlock()

	// check the content type request header
	contentTypeHeaders, ok := c.Request.Header["Content-Type"]
	if ok {
		// if the content type header is set, make sure it is exactly 'application/octet-stream'
		if len(contentTypeHeaders) > 1 || contentTypeHeaders[0] != "application/octet-stream" {

			// display an error to the user
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf(
					"Content-Type:%s is not supported. Don't set the header or use 'application/octet-stream'.",
					contentTypeHeaders[0],
				),
			})

			return
		}
	}

	includeCertificates := c.DefaultQuery("include-certificates", "none") != "none"

	// read request body
	query, err := io.ReadAll(c.Request.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading request body failed: %v\n", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	requestBitStringPairs, minAltitude, maxAltitude, err := comm.ParseQuery(query)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	sqlQuery := database.BuildNodeQuery(requestBitStringPairs, minAltitude, maxAltitude)

	rows, err := env.DbPool.Query(
		c.Request.Context(),
		sqlQuery,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "node query failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "database query failed",
		})
		return
	}

	defer rows.Close()

	// allocate slice with capacity of 'len(bit_strings)' and let go handle slice growth
	nodes, rootHash, certificateStringHashes, err := database.RowsToNodesAndRootHash(rows, len(requestBitStringPairs))
	if err != nil {
		fmt.Fprintf(os.Stderr, "scanning node rows failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "scanning rows failed",
		})
		return
	}

	if rootHash == nil {
		fmt.Fprintf(os.Stderr, "integrity check failed, root node was not returned by the query")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "integrity check failed, root node was not returned by the query",
		})
		return
	}

	var certificates [][]byte
	if includeCertificates && certificateStringHashes.Cardinality() > 0 {
		sqlQuery, err := database.BuildCertificateQuery(certificateStringHashes.ToSlice())
		if err != nil {
			fmt.Fprintf(os.Stderr, "building certificate query failed: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "building database query failed",
			})
			return
		}

		rows, err := env.DbPool.Query(
			c.Request.Context(),
			sqlQuery,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "certificate query failed: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "database query failed",
			})
			return
		}

		defer rows.Close()

		// allocate slice with capacity of 'certificateStringHashes.Cardinality()', let go handle slice growth
		certificates, err = database.RowsToCertificates(rows, certificateStringHashes.Cardinality())
		if err != nil {
			fmt.Fprintf(os.Stderr, "scanning certificate rows failed: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "scanning rows failed",
			})
			return
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
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed marshaling response",
		})
		return
	}

	c.Data(
		http.StatusOK,
		"application/octet-stream",
		response,
	)
}

// handler for the /certificates endpoint
func (env *EndpointHandlerEnv) getCertificates(c *gin.Context) {

	certificateStringHashes, nonEmpty := c.GetQueryArray("hash")
	if !nonEmpty {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "provide at least one hash using the 'hash' parameter",
		})
		return
	}

	var certificates [][]byte
	sqlQuery, err := database.BuildCertificateQuery(certificateStringHashes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "building certificate query failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "building database query failed",
		})
		return
	}

	rows, err := env.DbPool.Query(
		c.Request.Context(),
		sqlQuery,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "certificate query failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "database query failed",
		})
		return
	}

	defer rows.Close()

	// at least allocate a capacity of 'len(bit_strings)', then let
	// the go standard libary handle growth
	certificates, err = database.RowsToCertificates(rows, len(certificateStringHashes))
	if err != nil {
		fmt.Fprintf(os.Stderr, "scanning certificate rows failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "scanning rows failed",
		})
		return
	}

	response, err := proto.Marshal(&comm.Response{
		Certificates: certificates,
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "failed marshaling response: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed marshaling response",
		})
		return
	}

	c.Data(
		http.StatusOK,
		"application/octet-stream",
		response,
	)
}
