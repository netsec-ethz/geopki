package server

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"
)

func (env *EndpointHandlerEnv) getSignedConsistencyHead(c *gin.Context) {
	env.SharedDataLock.RLock()
	sch := env.CurrentSignedConsistencyHead
	env.SharedDataLock.RUnlock()

	response, err := proto.Marshal(sch)
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

func (env *EndpointHandlerEnv) getSignedMapHead(c *gin.Context) {
	env.SharedDataLock.RLock()
	smh := env.CurrentSignedMapHead
	env.SharedDataLock.RUnlock()

	response, err := proto.Marshal(smh)
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

func (env *EndpointHandlerEnv) getSignedConsistencyHeadConsistency(c *gin.Context) {

	treeSize1Str := c.DefaultQuery("first", "abc")
	treeSize2Str := c.DefaultQuery("second", "abc")

	treeSize1, err := strconv.ParseUint(treeSize1Str, 2, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "received invalid tree size for argument 'first'",
		})
		return
	}

	treeSize2, err := strconv.ParseUint(treeSize2Str, 2, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "received invalid tree size for argument 'second'",
		})
		return
	}

	if treeSize1 == treeSize2 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "the two tree sizes cannot be the same",
		})
		return
	}

	proof, err := env.ConsistencyClient.ConsistencyProof(c.Request.Context(), treeSize1, treeSize2)
	if err != nil {
		fmt.Fprintf(os.Stderr, "obtaining consistency proof failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "retrieving consistency proof failed",
		})
		return
	}

	response, err := proto.Marshal(proof)
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

func (env *EndpointHandlerEnv) getProofByHash(c *gin.Context) {

	hashBase64 := c.DefaultQuery("hash", "#")
	treeSizeStr := c.DefaultQuery("tree_size", "abc")

	treeSize, err := strconv.ParseUint(treeSizeStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "received invalid tree size for argument 'tree_size'",
		})
		return
	}

	hash, err := base64.RawURLEncoding.DecodeString(hashBase64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	proof, err := env.ConsistencyClient.ProveSignedMapHeadHashInclusion(c.Request.Context(), treeSize, hash)
	if err != nil {
		fmt.Fprintf(os.Stderr, "obtaining a proof of inclusion for consistency tree failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "obtaining a proof of inclusion for consistency tree failed",
		})
		return
	}

	response, err := proto.Marshal(proof)
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

func (env *EndpointHandlerEnv) getEntries(c *gin.Context) {
	startStr := c.DefaultQuery("start", "abc")
	endStr := c.DefaultQuery("end", "abc")

	start, err := strconv.ParseUint(startStr, 2, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "received invalid index for argument 'start'",
		})
		return
	}

	end, err := strconv.ParseUint(endStr, 2, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "received invalid index for argument 'end'",
		})
		return
	}

	if start > end {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "the 'end' value must be greater than or equal to 'start'",
		})
		return
	}

	entries, err := env.ConsistencyClient.GetEntries(c.Request.Context(), start, end)
	if err != nil {
		fmt.Fprintf(os.Stderr, "obtaining entries failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "obtaining entries failed",
		})
		return
	}

	response, err := proto.Marshal(entries)
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

func (env *EndpointHandlerEnv) getEntryAndProof(c *gin.Context) {
	leafIndexStr := c.DefaultQuery("leaf_index", "abc")
	treeSizeStr := c.DefaultQuery("tree_size", "abc")

	leafIndex, err := strconv.ParseUint(leafIndexStr, 2, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "received invalid index for argument 'leaf_index'",
		})
		return
	}

	treeSize, err := strconv.ParseUint(treeSizeStr, 2, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "received invalid tree size for argument 'tree_size'",
		})
		return
	}

	if leafIndex >= treeSize {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "the 'leaf_index' value must be strictly greater than 'tree_size'",
		})
		return
	}

	entryAndProof, err := env.ConsistencyClient.GetEntryAndProof(c.Request.Context(), treeSize, leafIndex)
	if err != nil {
		fmt.Fprintf(os.Stderr, "obtaining entry and proof failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "obtaining entry and proof failed",
		})
		return
	}

	response, err := proto.Marshal(entryAndProof)
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
