package server

import (
	"crypto/x509"
	"fmt"
	"net/http"
	"os"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
)

func StartServer(
	env *EndpointHandlerEnv,
	listenAddress string,
	listenPort uint64,
) {
	// setup web server
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	pprof.Register(r)

	// configure gin engine
	r.SetTrustedProxies(TRUSTED_PROXIES)

	// setup middlewares
	r.Use(gin.Recovery())
	// do not use compression, lowers the throughput
	// r.Use(gzip.Gzip(gzip.DefaultCompression))
	r.Use(cors.New(cors.Config{
		AllowAllOrigins:  true,
		AllowMethods:     []string{"GET", "POST", "HEAD"},
		AllowHeaders:     []string{"Content-Length", "Content-Type"},
		AllowCredentials: false,
	}))

	// install endpoints

	// queries
	r.POST("/v1/query", env.postQuery)
	r.GET("/v1/certificates", env.getCertificates)
	r.GET("/v1/public-key", env.getPublicKey)

	// consistency
	r.GET("/v1/get-sch", env.getSignedConsistencyHead)
	r.GET("/v1/get-smh", env.getSignedMapHead)
	r.GET("/v1/get-sch-consistency", env.getSignedConsistencyHeadConsistency)
	r.GET("/v1/get-proof-by-hash", env.getProofByHash)
	r.GET("/v1/get-entries", env.getEntries)
	r.GET("/v1/get-entry-and-proof", env.getEntryAndProof)

	// ingestion
	r.POST("/v1/insert", env.postInsert)
	r.POST("/v1/release", env.postRelaseNewVersion)

	// install demo endpoint
	r.Static("/demo", "./demo/geopki-web-client")

	// start server
	r.Run(fmt.Sprintf("%s:%d", listenAddress, listenPort))
}

// handler for the /public-key endpoint
func (env *EndpointHandlerEnv) getPublicKey(c *gin.Context) {
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

	publicKey, err := x509.MarshalPKIXPublicKey(&env.PrivateKey.PublicKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed marshaling public key: %v\n", err)

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed marshaling public key",
		})
		return
	}

	c.Data(
		http.StatusOK,
		"application/octet-stream",
		publicKey,
	)
}
