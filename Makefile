.PHONY: all test test-wasm

# The lukeroth/gdal cgo bindings wrap GDALGetDataTypeSize.
# With GDAL >= 3.5 it is marked as deprecated.
# We never call it, so silence the noise rather than patch the dependency.
# Appended to Go's own default CGO_CFLAGS instead of replacing it.
export CGO_CFLAGS := $(shell go env CGO_CFLAGS) -Wno-deprecated-declarations

# The packages that make up the wasm client, used to build the unit tests for WASM.
# Everything else requires the cgo GDAL bindings, which cannot be built for js/wasm.
WASM_PKGS := ./cmd/geopki-client-wasm ./pkg/bitstring ./pkg/comm ./pkg/crypto

all: client server wasm-client demo db-address-exporter client-performance client-throughput client-ingestion

client: ./cmd/geopki-client/geopki-client.go $(wildcard pkg/**/*)
	go build -o ./dist/geopki-client ./cmd/geopki-client

server: ./cmd/geopki-server/geopki-server.go $(wildcard pkg/**/*)
	go build -o ./dist/geopki-server ./cmd/geopki-server

wasm-client: ./cmd/geopki-client-wasm/geopki-client-wasm.go $(wildcard pkg/**/*)
	GOOS=js GOARCH=wasm go build -o ./dist/geopki-client.wasm ./cmd/geopki-client-wasm

demo: wasm-client
	cp ./dist/geopki-client.wasm ./demo/geopki-web-client/geopki-client.wasm

db-address-exporter: ./cmd/db-address-exporter/db-address-exporter.go $(wildcard pkg/**/*)
	go build -o ./dist/db-address-exporter ./cmd/db-address-exporter

client-performance: ./cmd/geopki-client-performance/geopki-client-performance.go $(wildcard pkg/**/*)
	go build -o ./dist/geopki-client-performance ./cmd/geopki-client-performance

client-throughput: ./cmd/geopki-client-throughput/geopki-client-throughput.go $(wildcard pkg/**/*)
	go build -o ./dist/geopki-client-throughput ./cmd/geopki-client-throughput

client-ingestion: ./cmd/geopki-client-ingestion/geopki-client-ingestion.go $(wildcard pkg/**/*)
	go build -o ./dist/geopki-client-ingestion ./cmd/geopki-client-ingestion


# The package cmd/geopki-client-wasm is js/wasm-only, so it can't be built regularly.
# It is excluded here and covered by test-wasm instead.
test:
	go test $$(go list -e ./... | grep -v geopki-client-wasm)

# Builds the wasm client and the packages it shares for js/wasm and runs their tests under node,
# via Go's go_js_wasm_exec helper. Requires node on PATH.
# The js/wasm runtime caps the combined size of argv and the environment,
# so this runs with a trimmed env rather than inheriting the caller's.
test-wasm:
	env -i \
	  PATH="/usr/bin:/bin:$$(go env GOROOT)/bin:$$(go env GOROOT)/lib/wasm" \
	  HOME="$$HOME" \
	  GOPATH="$$(go env GOPATH)" \
	  GOMODCACHE="$$(go env GOMODCACHE)" \
	  GOCACHE="$$(go env GOCACHE)" \
	  GOOS=js GOARCH=wasm \
	  go test $(WASM_PKGS)

clean:
	rm -f ./dist/geopki-client
	rm -f ./dist/geopki-server
	rm -f ./dist/geopki-client.wasm
	rm -f ./demo/geopki-web-client/geopki-client.wasm
	rm -f ./dist/db-address-exporter
	rm -f ./dist/client-performance
	rm -f ./dist/bitstring-throughput
	rm -f ./dist/bitstring-ingestion

find-leaks:
	# Uses gitleaks https://github.com/gitleaks/gitleaks
	gitleaks git -v
