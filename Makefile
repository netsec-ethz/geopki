# The lukeroth/gdal cgo bindings wrap GDALGetDataTypeSize.
# With GDAL >= 3.5 it is marked as deprecated.
# We never call it, so silence the noise rather than patch the dependency.
# Appended to Go's own default CGO_CFLAGS instead of replacing it.
export CGO_CFLAGS := $(shell go env CGO_CFLAGS) -Wno-deprecated-declarations

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
