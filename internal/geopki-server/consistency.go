package server

import (
	"encoding/base64"
	"fmt"
	"os"
	"strconv"

	"github.com/valyala/fasthttp"
	"google.golang.org/protobuf/proto"
)

func (env *EndpointHandlerEnv) getSignedConsistencyHead(ctx *fasthttp.RequestCtx) {
	env.SharedDataLock.RLock()
	sch := env.CurrentSignedConsistencyHead
	env.SharedDataLock.RUnlock()

	response, err := proto.Marshal(sch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed marshaling response: %v\n", err)
		errorHandler(ctx, fasthttp.StatusInternalServerError, "failed marshaling response")
		return
	}

	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBody(response)
}

func (env *EndpointHandlerEnv) getSignedMapHead(ctx *fasthttp.RequestCtx) {
	env.SharedDataLock.RLock()
	smh := env.CurrentSignedMapHead
	env.SharedDataLock.RUnlock()

	response, err := proto.Marshal(smh)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed marshaling response: %v\n", err)
		errorHandler(ctx, fasthttp.StatusInternalServerError, "failed marshaling response")
		return
	}

	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBody(response)
}

func (env *EndpointHandlerEnv) getSignedConsistencyHeadConsistency(ctx *fasthttp.RequestCtx) {
	args := ctx.QueryArgs()
	treeSize1Str := args.Peek("first")
	treeSize2Str := args.Peek("second")

	if treeSize1Str == nil {
		errorHandler(ctx, fasthttp.StatusBadRequest, "query argument 'first' is missing")
		return
	}

	if treeSize2Str == nil {
		errorHandler(ctx, fasthttp.StatusBadRequest, "query argument 'second' is missing")
		return
	}

	treeSize1, err := strconv.ParseUint(string(treeSize1Str), 2, 64)
	if err != nil {
		errorHandler(ctx, fasthttp.StatusBadRequest, "received invalid tree size for argument 'first'")
		return
	}

	treeSize2, err := strconv.ParseUint(string(treeSize2Str), 2, 64)
	if err != nil {
		errorHandler(ctx, fasthttp.StatusBadRequest, "received invalid tree size for argument 'second'")
		return
	}

	if treeSize1 == treeSize2 {
		errorHandler(ctx, fasthttp.StatusBadRequest, "the two tree sizes cannot be the same")
		return
	}

	proof, err := env.ConsistencyClient.ConsistencyProof(ctx, treeSize1, treeSize2)
	if err != nil {
		fmt.Fprintf(os.Stderr, "obtaining consistency proof failed: %v\n", err)
		errorHandler(ctx, fasthttp.StatusInternalServerError, "retrieving consistency proof failed")
		return
	}

	response, err := proto.Marshal(proof)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed marshaling response: %v\n", err)
		errorHandler(ctx, fasthttp.StatusInternalServerError, "failed marshaling response")
		return
	}

	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBody(response)
}

func (env *EndpointHandlerEnv) getProofByHash(ctx *fasthttp.RequestCtx) {
	args := ctx.QueryArgs()
	hashBase64 := args.Peek("hash")
	treeSizeStr := args.Peek("tree_size")

	if hashBase64 == nil {
		errorHandler(ctx, fasthttp.StatusBadRequest, "query argument 'hash' is missing")
		return
	}

	if treeSizeStr == nil {
		errorHandler(ctx, fasthttp.StatusBadRequest, "query argument 'tree_size' is missing")
		return
	}

	treeSize, err := strconv.ParseUint(string(treeSizeStr), 10, 64)
	if err != nil {
		errorHandler(ctx, fasthttp.StatusBadRequest, "received invalid tree size for argument 'tree_size'")
		return
	}

	hash := make([]byte, base64.RawURLEncoding.DecodedLen(len(hashBase64)))
	n, err := base64.RawURLEncoding.Decode(hash, hashBase64)
	hash = hash[:n]
	if err != nil {
		errorHandler(ctx, fasthttp.StatusBadRequest, err.Error())
		return
	}

	proof, err := env.ConsistencyClient.ProveSignedMapHeadHashInclusion(ctx, treeSize, hash)
	if err != nil {
		fmt.Fprintf(os.Stderr, "obtaining a proof of inclusion for consistency tree failed: %v\n", err)
		errorHandler(ctx, fasthttp.StatusInternalServerError, "obtaining a proof of inclusion for consistency tree failed")
		return
	}

	response, err := proto.Marshal(proof)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed marshaling response: %v\n", err)
		errorHandler(ctx, fasthttp.StatusInternalServerError, "failed marshaling response")
		return
	}

	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBody(response)
}

func (env *EndpointHandlerEnv) getEntries(ctx *fasthttp.RequestCtx) {
	args := ctx.QueryArgs()
	startStr := args.Peek("start")
	endStr := args.Peek("end")

	if startStr == nil {
		errorHandler(ctx, fasthttp.StatusBadRequest, "query argument 'start' is missing")
		return
	}

	if endStr == nil {
		errorHandler(ctx, fasthttp.StatusBadRequest, "query argument 'end' is missing")
		return
	}

	start, err := strconv.ParseUint(string(startStr), 2, 64)
	if err != nil {
		errorHandler(ctx, fasthttp.StatusBadRequest, "received invalid index for argument 'start'")
		return
	}

	end, err := strconv.ParseUint(string(endStr), 2, 64)
	if err != nil {
		errorHandler(ctx, fasthttp.StatusBadRequest, "received invalid index for argument 'end'")
		return
	}

	if start > end {
		errorHandler(ctx, fasthttp.StatusBadRequest, "the 'end' value must be greater than or equal to 'start'")
		return
	}

	entries, err := env.ConsistencyClient.GetEntries(ctx, start, end)
	if err != nil {
		fmt.Fprintf(os.Stderr, "obtaining entries failed: %v\n", err)
		errorHandler(ctx, fasthttp.StatusInternalServerError, "obtaining entries failed")
		return
	}

	response, err := proto.Marshal(entries)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed marshaling response: %v\n", err)
		errorHandler(ctx, fasthttp.StatusInternalServerError, "failed marshaling response")
		return
	}

	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBody(response)
}

func (env *EndpointHandlerEnv) getEntryAndProof(ctx *fasthttp.RequestCtx) {
	args := ctx.QueryArgs()

	leafIndexStr := args.Peek("leaf_index")
	treeSizeStr := args.Peek("tree_size")

	if leafIndexStr == nil {
		errorHandler(ctx, fasthttp.StatusBadRequest, "query argument 'leaf_index' is missing")
		return
	}

	if treeSizeStr == nil {
		errorHandler(ctx, fasthttp.StatusBadRequest, "query argument 'tree_size' is missing")
		return
	}

	leafIndex, err := strconv.ParseUint(string(leafIndexStr), 2, 64)
	if err != nil {
		errorHandler(ctx, fasthttp.StatusBadRequest, "received invalid index for argument 'leaf_index'")
		return
	}

	treeSize, err := strconv.ParseUint(string(treeSizeStr), 2, 64)
	if err != nil {
		errorHandler(ctx, fasthttp.StatusBadRequest, "received invalid tree size for argument 'tree_size'")
		return
	}

	if leafIndex >= treeSize {
		errorHandler(ctx, fasthttp.StatusBadRequest, "the 'leaf_index' value must be strictly greater than 'tree_size'")
		return
	}

	entryAndProof, err := env.ConsistencyClient.GetEntryAndProof(ctx, treeSize, leafIndex)
	if err != nil {
		fmt.Fprintf(os.Stderr, "obtaining entry and proof failed: %v\n", err)
		errorHandler(ctx, fasthttp.StatusInternalServerError, "obtaining entry and proof failed")
		return
	}

	response, err := proto.Marshal(entryAndProof)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed marshaling response: %v\n", err)
		errorHandler(ctx, fasthttp.StatusInternalServerError, "failed marshaling response")
		return
	}

	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBody(response)
}
