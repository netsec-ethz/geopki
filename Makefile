all: client server wasm-client demo db-address-exporter bitstring-performance client-performance client-throughput

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

bitstring-performance: ./cmd/bitstring-performance/bitstring-performance.go $(wildcard pkg/**/*)
	go build -o ./dist/bitstring-performance ./cmd/bitstring-performance

client-performance: ./cmd/geopki-client-performance/geopki-client-performance.go $(wildcard pkg/**/*)
	go build -o ./dist/geopki-client-performance ./cmd/geopki-client-performance

client-throughput: ./cmd/geopki-client-throughput/geopki-client-throughput.go $(wildcard pkg/**/*)
	go build -o ./dist/geopki-client-throughput ./cmd/geopki-client-throughput

clean:
	rm -f ./dist/geopki-client
	rm -f ./dist/geopki-server
	rm -f ./dist/geopki-client.wasm
	rm -f ./demo/geopki-web-client/geopki-client.wasm
	rm -f ./dist/db-address-exporter
	rm -f ./dist/bitstring-performance
	rm -f ./dist/client-performance
	rm -f ./dist/bitstring-throughput