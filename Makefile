all: client server wasm-client demo bitstring-performance db-address-exporter

client: ./cmd/geopki-client/geopki-client.go $(wildcard pkg/**/*)
	go build -o ./dist/geopki-client ./cmd/geopki-client

server: ./cmd/geopki-server/geopki-server.go $(wildcard pkg/**/*)
	go build -o ./dist/geopki-server ./cmd/geopki-server

wasm-client: ./cmd/geopki-client-wasm/geopki-client-wasm.go $(wildcard pkg/**/*)
	GOOS=js GOARCH=wasm go build -o ./dist/geopki-client.wasm ./cmd/geopki-client-wasm
	
demo: wasm-client
	cp ./dist/geopki-client.wasm ./demo/geopki-web-client/geopki-client.wasm

bitstring-performance: ./cmd/bitstring-performance/bitstring-performance.go $(wildcard pkg/**/*)
	go build -o ./dist/bitstring-performance ./cmd/bitstring-performance

db-address-exporter: ./cmd/db-address-exporter/db-address-exporter.go $(wildcard pkg/**/*)
	go build -o ./dist/db-address-exporter ./cmd/db-address-exporter

clean:
	rm -f ./dist/geopki-client ./dist/geopki-server ./dist/geopki-client.wasm ./demo/geopki-web-client/geopki-client.wasm ./dist/bitstring-performance ./dist/db-address-exporter