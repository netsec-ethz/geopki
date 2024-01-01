package server

import (
	"crypto/x509"
	"fmt"
	"os"
	"strings"

	"github.com/valyala/fasthttp"
)

func (env *EndpointHandlerEnv) newRequestHandler(
	demoHandler fasthttp.RequestHandler,
) fasthttp.RequestHandler {

	return func(ctx *fasthttp.RequestCtx) {
		// CORS headers
		ctx.Response.Header.Set("Access-Control-Allow-Origin", "*")
		ctx.Response.Header.Set("Access-Control-Allow-Methods", "GET,POST,HEAD")
		ctx.Response.Header.Set("Access-Control-Allow-Headers", "Content-Length, Content-Type")
		ctx.Response.Header.Set("Access-Control-Max-Age", "86400")
		// terminate pre-flight requests
		if ctx.IsOptions() {
			ctx.SetStatusCode(fasthttp.StatusNoContent)
			return
		}

		switch string(ctx.Path()) {
		// queries
		case "/v1/query":
			if ctx.IsPost() {
				env.postQuery(ctx)
				return
			}
			notFoundHandler(ctx)

		case "/v1/certificates":
			if ctx.IsGet() {
				env.getCertificates(ctx)
				return
			}
			notFoundHandler(ctx)

		case "/v1/public-key":
			if ctx.IsGet() {
				env.getPublicKey(ctx)
				return
			}
			notFoundHandler(ctx)

		// consistency
		case "/v1/get-sch":
			if ctx.IsGet() {
				env.getSignedConsistencyHead(ctx)
				return
			}
			notFoundHandler(ctx)

		case "/v1/get-smh":
			if ctx.IsGet() {
				env.getSignedMapHead(ctx)
				return
			}
			notFoundHandler(ctx)

		case "/v1/get-sch-consistency":
			if ctx.IsGet() {
				env.getSignedConsistencyHeadConsistency(ctx)
				return
			}
			notFoundHandler(ctx)

		case "/v1/get-proof-by-hash":
			if ctx.IsGet() {
				env.getProofByHash(ctx)
				return
			}
			notFoundHandler(ctx)

		case "/v1/get-entries":
			if ctx.IsGet() {
				env.getEntries(ctx)
				return
			}
			notFoundHandler(ctx)

		case "/v1/get-entry-and-proof":
			if ctx.IsGet() {
				env.getEntryAndProof(ctx)
				return
			}
			notFoundHandler(ctx)

		// ingestion
		case "/v1/insert":
			if ctx.IsPost() {
				env.postInsert(ctx)
				return
			}
			notFoundHandler(ctx)

		case "/v1/release":
			if ctx.IsPost() {
				env.postRelaseNewVersion(ctx)
				return
			}
			notFoundHandler(ctx)

		case "/demo":
			ctx.Redirect("/demo/", fasthttp.StatusMovedPermanently)

		default:
			p := string(ctx.Path())
			if strings.HasPrefix(p, "/demo/") && ctx.IsGet() {
				// strip /demo
				ctx.URI().SetPath(string(ctx.Path()[6:]))
				demoHandler(ctx)
				return
			}

			notFoundHandler(ctx)
		}
	}

}

func StartServer(
	env *EndpointHandlerEnv,
	listenAddress string,
	listenPort uint64,
) {

	// install demo endpoint
	// Setup FS handler
	fs := &fasthttp.FS{
		Root:               "./demo/geopki-web-client",
		IndexNames:         []string{"index.html"},
		GenerateIndexPages: false,
		Compress:           true,
		AcceptByteRange:    true,
	}

	requestHandler := env.newRequestHandler(
		fs.NewRequestHandler(),
	)

	// setup web server
	fasthttp.ListenAndServe(
		fmt.Sprintf("%s:%d", listenAddress, listenPort),
		requestHandler,
	)
}

// handler for the /public-key endpoint
func (env *EndpointHandlerEnv) getPublicKey(ctx *fasthttp.RequestCtx) {
	publicKey, err := x509.MarshalPKIXPublicKey(&env.PrivateKey.PublicKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed marshaling public key: %v\n", err)
		errorHandler(ctx, fasthttp.StatusBadRequest, "failed marshaling public key")
		return
	}

	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBody(publicKey)
}
