# geopki

Prototype implementation for a PKI extension that allows the querying of the set of all certificates associated with a given geographical area.
This is analogous to [F-PKI](https://netsec.ethz.ch/publications/papers/chuat2022fpki.pdf) which provides the same functionality for domains, i.e. allows the querying of the set of all certificates for a given domain.

## Table of Contents

- [geopki](#geopki)
  - [Table of Contents](#table-of-contents)
  - [Project Structure](#project-structure)
    - [Go Server \& Client](#go-server-client)
    - [Bitstring Computation Performance Measurements](#bitstring-computation-performance-measurements)
    - [Database Input](#database-input)
    - [Database Performance Measurements](#database-performance-measurements)
    - [Full System Performance Measurements](#full-system-performance-measurements)
  - [Setup \& Usage](#setup-usage)
    - [Prerequisites](#prerequisites)
      - [Clone Repo](#clone-repo)
      - [Go](#go)
      - [Python](#python)
      - [PostgreSQL](#postgresql)
      - [Trillian](#trillian)
    - [GeoPKI Server](#geopki-server)
      - [Quick Copy\&Paste](#quick-copypaste)
    - [Import Data](#import-data)
      - [Release](#release)
      - [Truncate Database](#truncate-database)
      - [Sample](#sample)
      - [World](#world)
        - [Alternative in Python](#alternative-in-python)
    - [GeoPKI Client](#geopki-client)
      - [Web Demo Client](#web-demo-client)
    - [Errors](#errors)
      - [Incompatible GDAL Setup](#incompatible-gdal-setup)
  - [Performance Evaluation](#performance-evaluation)
    - [Sampling Maps](#sampling-maps)
      - [performance/sampling-map-generator.py](#performancesampling-map-generatorpy)
      - [performance/sampling-map-converter.py](#performancesampling-map-converterpy)
    - [Database Only](#database-only)
      - [Spatial Index](#spatial-index)
      - [Prefix Matching](#prefix-matching)
      - [Integer Range Query](#integer-range-query)
      - [Depths](#depths)
        - [Certificate Depths](#certificate-depths)
        - [Sparse SMT Node Depths](#sparse-smt-node-depths)
      - [Altitude Distribution](#altitude-distribution)
      - [Plotting the Results](#plotting-the-results)
      - [Plotting Certificate Size Distribution](#plotting-certificate-size-distribution)
    - [Full System](#full-system)
      - [Metrics on Individual Requests and Responses](#metrics-on-individual-requests-and-responses)
        - [Plotting the Results](#plotting-the-results-1)
      - [Throughput](#throughput)
        - [Plotting the Results](#plotting-the-results-2)
      - [Ingestion](#ingestion)
        - [Plotting the Results](#plotting-the-results-3)
        - [Release](#release-1)

## Project Structure

### Go Server & Client

Go Files, relevant for the server and client implementation

```text
.
├── cmd                           # the go programs
|   ├── geopki-client                 # program for querying a geopki server
|   └── geopki-server                 # the geopki server program
|
├── database                      # SQL table schemes and function definitions
|   ├── constraints                   # contraints for ensuring the consistency of the database
|   |   └── node-integrity.sql        # ensures the integrity of all node rows
|   |
|   ├── functions                     # required and useful SQL functions
|   |   └── ...                           # set of functions for a scheme with z subtrees
|   |
|   └── tables                         # SQL tables schemas
|       ├── certificates.sql               # the table schema for the certificates table
|       └── nodes.sql                      # the table schema for the nodes table
|
├── internal                      # code internal to this project, not exposing any useful APIs for other projects
|   └── geopki-server                 # internal code related to the server
|       ├── consistency.go                # server endpoints serving the consistency tree
|       ├── constants.go                  # server constants such as the relative grid size and the MMD
|       ├── ingestion.go                  # server endpoints for ingestion of new certificates
|       ├── query.go                      # main server endpoints for queries
|       ├── server.go                     # code for bootstrapping the server
|       ├── state.go                      # defines the struct for keeping server state
|       └── utilities.go                  # helper functions to issue new SMHs or to verify the insertion key
|
├── pkg                           # code potentially re-usable for projects building on this one
|   ├── bitstring                     # code related to the bit strings
|   |   ├── bit_string_pair.go            # code for a bit string object supporting higher level operations
|   |   └── raw_bit_string_pair.go        # code for a more low level bit string object supporting
|   |                                     # operations directly on its bits
|   ├── comm                          # communication related code
|   |   ├── query.go                      # code building a query client-side and parsing it server-side
|   |   ├── request.pb.go                 # protobuf query request message (./proto/request.proto)
|   |   └── response.pb.go                # protobuf query response message (./proto/response.proto)
|   |
|   ├── crypto                        # cryptography related code
|   |   ├── certificate.go                # model of the certificate object, currently JSON
|   |   ├── consistency.go                # code for proving the consistency of the map server
|   |   ├── hash.go                       # hashing related code, at the moment mostly just the default hash value
|   |   ├── node.go                       # in-memory representation of a tree node corresponding to a DB row
|   |   ├── smh.go                        # signed map head object supporting signing and signature verification
|   |   └── verification.go               # code for verifying server responses client-side
|   └── database                      # helper functions for accessing the database
|       ├── ingestion.go                  # code for ingesting new data but also removing expired certificates
|       └── query.go                      # code querying the database for SMT nodes and certificates
|
├── proto                         # protobuf messages
|   ├── request.proto                 # protobuf message for a query request sent by a client
|   └── response.proto                # protobuf message for a query response sent by a server
|
├── proto                         # samples
|   ├── ingestion_sample.py           # python script to easily add single geo certs into the DB
|   └── certs/                        # directory with real sample certificate chains
|
├── go.mod                        # standard go dependency managment file
└── go.sum                        # standard go dependency managment file
```

### Bitstring Computation Performance Measurements

```text
.
├── cmd                           # the go programs
|   └── bitstring                     # program for measuring the performance of the bit string computations given a volume
|
└── performance
    └── bitstring-performance         # measures the performance for approximating a sphere using bitstrings
        ├── bitstring-performance.py      # measures the performancy by calling cmd/bitstring-performance
        ├── bitstring-performance-plot.py # plots the output of bitstring-performancy.py (bitstring-performance.csv)
        ├── bitstring-performance.csv     # output of of bitstring-performancy.py
        ├── f-count.png                   # plot output of of bitstring-performance-plot.py
        └── f-time.png                    # plot output of of bitstring-performance-plot.py
```

### Database Input

```text
└── cmd                           # the go programs
    └── db-address-exporter       # generates SQL INSERT files based on the dataset in
                                  # https://gitlab.inf.ethz.ch/OU-PERRIG/theses/msc_nico_hauser/data/-/tree/main/osm-dataset
```

```text
database/scripts/input
├── db-input-analyzer.py          # plots the number of bit strings used for DBs with and without z subtrees
├── db-input-visualizer.py        # plots the polygon of a specific OSM location
└── db-sanity-check-generator.py  # generates sql files for a dummy table to verify the measurements
```

```text
database/scripts/input
├── db-address-exporter.py        # generates SQL INSERT files based on the dataset in
|                                 # https://gitlab.inf.ethz.ch/OU-PERRIG/theses/msc_nico_hauser/data/-/tree/main/osm-dataset
|                                 # *requires a lot of RAM*, worked with 64GB
├── db-input-analyzer.py          # plots the number of bit strings used for DBs with and without z subtrees
├── db-sanity-check-generator.py  # generates sql files for a dummy table to verify the measurements
└── graph-db-address-exporter.py  # generates CSV files that can be imported into neo4j 
```

### Database Performance Measurements

```text
database/scripts/performance
├── db-types                      # the scripts for running performance tests on various schemes
|   ├── postgres-baseline-run.py                  # performance test on postgres, baseline table
|   ├── postgres-bitstring-int-run.py             # performance test on postgres using integer range queries
|   ├── postgres-bitstring-txt-run.py             # performance test on postgres using prefix matching
|   └── postgres-spatial-run.py                   # performance test on postgres using a spatial index
|
└── performance-evaluation.py     # runs one script in db-types several times and collects the results
```

### Full System Performance Measurements

```text
.
├── cmd                           # the programs used by the performance measurement scripts
|   ├── geopki-client-ingestion                   # inserts the passed certificates and measures the time
|   ├── geopki-client-performance                 # collect metrics on sequential requests
|   ├── geopki-client-throughput                  # spawns multiple goroutines and sends many parallel requests
|   ├── geopki-public-key                         # retrieves the public key of a geopki-server
|   └── release                                   # initiates a table swap after the ingestion of new certificates
|
└── performance                   # scripts for measuring the performance
    ├── client                                    # collects metrics on sequential requests
    |   ├── client-performance.py                     # runs the performance test
    |   └── client-performance-plot.py                # plots the result
    |
    ├── ingestion                                 # measures the time it takes to ingest new certificates
    |   ├── ingestion.py                              # runs the performance test
    |   └── ingestion-plot.py                         # plots the result
    |
    ├── throughput                                 # measures the number of parallel requests the server can process
    |   ├── throughput.py                              # runs the performance test
    |   └── throughput-plot.py                         # plots the result
    |
    ├── sampling-map-generator.py                  # generates .parquet sampling maps
    └── sampling-map-converter.py                  # converts .parquet sampling maps to .json sampling maps
```

## Setup & Usage

### Prerequisites

#### Clone Repo

Clone the [GeoPKI repo](https://gitlab.inf.ethz.ch/OU-PERRIG/theses/msc_nico_hauser/geopki). We assume the directory is called `geopki`.

#### Go

For running the geopki server or example client, GoLang is required.

GoLang 1.19 is minimum.  
GoLang 1.21 gives GDAL issues, see [errors](#errors).

Install GoLang 1.19 or 1.20:
At the time of writing (2023-11-25) on Ubuntu 22, Aptitude delivers GoLang 1.18, Snap delivers 1.21.
Do: `snap install golang-go` (gives GoLang 1.21) and `snap refresh go --channel=1.20/stable` (downgrades to 1.20).

Most dependencies are automatically downloaded and managed by GoLang.
The only exception is the [GDAL (Geospatial Data Abstraction Library)](https://gdal.org/) which has to be installed seperately (see below).
[Go bindings](https://github.com/lukeroth/gdal) are used to interact with GDAL unless with the exception of the wasm client that uses [S2](https://github.com/golang/geo).

1. GDAL
   1. `apt install libgdal-dev`
   2. `apt install gdal-bin`
      - actually necessary ???
2. GDAL GoLang: `go get github.com/lukeroth/gdal`

#### Python

For running the Python scripts, Python and the relevant dependencies have to be installed. It is recommended to do so in a Python venv.x
Note that Python is **not** required to run the geopki client nor the server but scripts such as the performance measurements are written in Python.

1. add repo: `add-apt-repository ppa:deadsnakes/ppa`
2. install Python: `apt install python3.11`
   - tested on version `3.11`, should probably work on later versions
3. upgrade pip: `pip3 install --upgrade pip`
4. install venv: `apt install python3.11-venv`
5. create venv: `python3.11 -m venv venv`
6. activate venv: `source ./venv/bin/activate`
   - `deactivate` to deavtivate venv
7. install requirements (in venv): `pip install -r requirements.txt`

#### PostgreSQL

1. install PostgreSQL: `apt install postgresql`
2. start PostgreSQL: `systemctl start postgresql`
3. create user in Postgres
   1. connect to Postgres as default admin user: `sudo -u postgres psql`
   2. `CREATE USER geopki PASSWORD 'geopkiPW';`
4. create database
   1. `CREATE DATABASE geopki;`
   2. assign DB to new user (actually requried?) `ALTER DATABASE geopki OWNER TO geopki;`
5. run commands from files  
   `sudo -u postgres psql -d geopki -a -f <file.sql>`
   1. define functions
      - run `database/functions/array_difference.sql`
      - run `database/functions/array_intersect.sql`
      - run `database/functions/array_union.sql`
      - run `database/functions/update_children_hashes.sql`
      - run `database/functions/min_altitude_of_bit_string.sql`
      - run `database/functions/max_altitude_of_bit_string.sql`
      - run `database/functions/smt_hash.sql` (for release)
   2. define tables
      - run `database/tables/certificates.sql` to create the `certificates` table
      - run `database/tables/nodes.sql` to create the `nodes` table
      - fix permissions:
        - `ALTER TABLE certificates OWNER TO geopki;`
        - `ALTER TABLE nodes OWNER TO geopki;`
        - `ALTER TABLE nodes_next OWNER TO geopki;`

#### Trillian

Setup a [Trillian](https://github.com/google/trillian) instance.

1. Use the easy [local deployment with Docker](https://github.com/google/trillian/tree/v1.5.2/examples/deployment#local-deployments)
   1. clone repo into a separate directory (not `geopki`): `git clone https://github.com/google/trillian.git`
   2. `cd trillian`
   3. switch to version 1.5.2: `git checkout v1.5.2`
      - (didn't test newer versions)
   4. set a random password: `export MYSQL_ROOT_PASSWORD="$(openssl rand -hex 16)"`
      - (actually requried for local test?)
   5. start docker-compose: `docker-compose -f examples/deployment/docker-compose.yml up`
      - requries sudo or Docker priviledges
2. Create a new tree: `go run ./cmd/createtree --admin_server=127.0.0.1:8090` (in `trillian` directory)
3. Copy the log id from the output
   - The output contains a line like `Initialised Log (4361164615329344703) with new SignedTreeHead` where `4361164615329344703` corresponds to the log id.

### GeoPKI Server

1. In the cloned geopki directory, run: `make`
2. (if not running already): In a secondary Terminal: run Trillian: `sudo docker-compose -f examples/deployment/docker-compose.yml up`
3. Define environment variables
   1. `DATABASE_URL`
      - Set the database connection parameters for the Postgres database: `export DATABASE_URL=postgresql://<username>:<password>@127.0.0.1:5432/<postgres database>` replacing the `<>` with values.
      - e.g. `export DATABASE_URL=postgresql://geopki:geopkiPW@127.0.0.1:5432/geopki`
   2. `PRIVATE_KEY`
      - private key of GeoPKI server used to sign the map heads and consistency heads
      - The format is base64 encoded bytes that are accepted by go's `ParseECPrivateKey` function which according to the documentation means any EC private key in SEC 1, ASN.1 DER form.
      - The easiest way to obtain a correctly formatted value is to run the server once without having the value set. The server will then print a randomly generated key to the console.
      - e.g. `export PRIVATE_KEY=MHcCAQEEILoQhmCA33qdj8EkpxXlaa9+vc30MCyJBck4ARgVIgPWoAoGCCqGSM49AwEHoUQDQgAEpyVhZ/HNRKVdp/jwnjM9xEagSMsB6VA07ftQ2jxResYL+JujTfb8YPKGDCykTP+ccJ+OYoNFUzYJd7niuXBsDA==`
   3. `CERT_INSERT_KEY`
      - The server exposes the endpoint `/v1/insert` for showcasing the ingestion and deletion functionality. Since this endpoint causes a lot of computation and allows the insertion of arbitrary data at the moment, it is protected by a secret. In the future no such endpoint should exist and the map server should on its own crawl the CT logs.
      - Needed even if you don't want to use this functionality.
      - e.g. `export CERT_INSERT_KEY=abc`
      - Note: There has to be some sort of length limit or other limitation on this, as the following will be accepted, but won't work. comparison will always fail. `MHcCAQEEIJ6Z1Wqd/PTXbc9D3tBaJz3FwFk0D+QNXxkbm4UX1tSwoAoGCCqGSM49AwEHoUQDQgAEJvP8VNjGv1W2qne8NXUXlWlSUbUsz795+x1ilPtg2SADjpR8we9EeI5SMz8eiSfdG8HFS/lT87OZEYYI0ZBTDw==`
4. Run the server: `go run ./cmd/geopki-server --address=0.0.0.0 --port=1234 --trillian-address=127.0.0.1:8090 --clog-id=<trillian log id>`.
   - All arguments except `clog-id` can be omitted if the just shown default values should be used.
   - If you forgot the log id, you can use `cmd/list-trees` (in the geopki directory) to list all trees: `go run ./cmd/list-trees --admin_server=127.0.0.1:8090`
   - e.g. `go run ./cmd/geopki-server --address=0.0.0.0 --port=1234 --trillian-address=127.0.0.1:8090 --clog-id=4361164615329344703`.
     - adapt the `--clog-id`

Import data, see [below](#import-data).

#### Quick Copy&Paste

If everything is set up, run these commands to start the geo map server:

Terminal 1, Trillian:

```bash
sudo docker-compose -f examples/deployment/docker-compose.yml up
```

Terminal 2, GeoPKI (adapt the `--clog-id`):

```bash
export DATABASE_URL=postgresql://geopki:geopkiPW@127.0.0.1:5432/geopki
export PRIVATE_KEY=MHcCAQEEILoQhmCA33qdj8EkpxXlaa9+vc30MCyJBck4ARgVIgPWoAoGCCqGSM49AwEHoUQDQgAEpyVhZ/HNRKVdp/jwnjM9xEagSMsB6VA07ftQ2jxResYL+JujTfb8YPKGDCykTP+ccJ+OYoNFUzYJd7niuXBsDA==
export CERT_INSERT_KEY=abc
go run ./cmd/geopki-server --address=0.0.0.0 --port=1234 --trillian-address=127.0.0.1:8090 --clog-id=4361164615329344703
```

Helpful commands for playing around:

To connect to the DB: `sudo -u postgres psql geopki`

To run SQL commands from file: `sudo -u postgres psql -d geopki -a -f <file.sql>`

### Import Data

After importing data, you must do a [release](#release) to update GeoPKI's working dataset.

#### Release

A release updates GeoPKI's working set. Run:

```bash
go run ./cmd/release \
    --address=http://127.0.0.1:1234 \
    --insertion-key=abc
```

#### Truncate Database

If you want to start from an empty database, truncate all tables.

WARNING: This deletes all data from the database!

```bash
sudo -u postgres psql -d geopki -a -f 'sample/truncate-db.sql'
```

Then, do a [release](#release). Or import new data first, and then do a release.

#### Sample

Use `./sample/ingestion_sample.py` to import a single certificate.
The values (geographic location of the certificate, domain, payload, etc.) can be easily customized by slightly adapting the script.

This script does a [release](#release) automatically.

#### World

Clone the [data repo](https://gitlab.inf.ethz.ch/OU-PERRIG/theses/msc_nico_hauser/data) (large!) or download individual parts. The full dataset is [world.parquet](https://gitlab.inf.ethz.ch/OU-PERRIG/theses/msc_nico_hauser/data/-/blob/main/osm-dataset/world.parquet).

Use the binary `./cmd/db-address-exporter`.

Parameters:

- `--input` to specify the `.parquet` certificate dataset
- `--nodes`: output folder for `.sql` files that insert data into the `nodes` table. Directory must exist.
- `--certs`: output folder for `.sql` files that insert data into the `certificates` table. Directory must exist.
- `--cert-size-dist` to specify the path where the `.csv` certificate size distribution should be written
- `--altitude-dist` to specify the path where the `.csv` altitude distribution of certificates should be written

The import procedure takes >1h, uses all memory and sometimes goes to 100% CPU usage.
Run it inside a separate terminal, an IDE terminal won't work as the high memory usage will crash the IDE (incl. the terminal).

```bash
go run ./cmd/db-address-exporter/ \
    --input=../geopki-data/world.parquet \
    --nodes=../geopki-data/db-import/nodes/ \
    --certs=../geopki-data/db-import/certificates/ \
    --sampling-map=./performance/sampling-map.csv \
    --cert-size-dist=./performance/cert-sizes/cert-sizes.csv \
    --altitude-dist=./database/measurements/altitude.csv
```

Then, do a [release](#release).

##### Alternative in Python

Alternatively, the script `./database/scripts/input/db-address-exporter.py` can be used for the same purpose.
In constrast, this script is slower, consumes more RAM and only writes sql files.
Therefore, for importing data for the actual system, this script is not recommended.

Howerver, for measuring the performance of the different indices, this script is the only way to generate the data in the respective format.

The script accepts three positional arguments: The input path pointing to the OSM `.parquet` certificates dataset and two output paths, one for the `.sql` files for the `nodes` table and one for the `certificates` table.

It then accepts an optional `--spatial` flag that generates the data for the [evaluation of the spatial index](#spatial-index).
If the flag is omitted, the database format for integer range queries is generated.

Example:

```bash
source ./venv/bin/activate
python3.11 database/scripts/input/db-address-exporter.py \
    ../geopki-data/world.parquet \
    ../geopki-data/db-import/nodes/ \
    ../geopki-data/db-import/certificates/ \
    --spatial
```

Then, do a [release](#release).

### GeoPKI Client

1. build client: `make client`
2. optain public key of GeoPKI server's private key
   - from server
     - `go run ./cmd/geopki-public-key -address='http://127.0.0.1:1234'`
   - locally
     1. save base64 encoded public key from server in file `private.base64`
     2. base64 decode: `openssl base64 -d -in private.base64 -out private.der`
     3. get public key: `openssl ec -in private.der -pubout -out public.der -inform der -outform der`
     4. encode public key in base64: `openssl base64 -in public.der -out public.base64`
     5. public key in base64 is in file `public.base64`
3. make client request using `./dist/geopki-client`
   - parameters:
     - `-latitude` / `-longitude`: latitude/longitude in degrees
     - `-altitude`: meters above sea level
     - `-radius`: meters
     - `-include-certificates`: Boolean: False: return only certificate hashes. True: Return full certificates.
     - `-address`: GeoPKI server address
     - `-public-key` The GeoPKI server's public key.
   - example using the keys used in the setup:  
   ```bash
   ./dist/geopki-client \
       --latitude=0.0 \
       --longitude=0.0 \
       --altitude=0.0 \
       --radius=100 \
       --include-certificates=true \
       --address='http://127.0.0.1:1234' \
       --public-key='MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEpyVhZ/HNRKVdp/jwnjM9xEagSMsB6VA07ftQ2jxResYL+JujTfb8YPKGDCykTP+ccJ+OYoNFUzYJd7niuXBsDA=='
   ```

#### Web Demo Client

1. `make demo`
   - Compiles `./cmd/geopki-client-wasm` and copies the binary to `./demo/geopki-web-client/geopki-client.wasm`.
2. Run an HTTP server that serves `./demo/geopki-web-client`.
   - e.g. locally for testing: `python3 -m http.server 9000 --directory ./demo/geopki-web-client`
3. open [localhost:9000](localhost:9000) (not 0.0.0.0, there the geo location prompt won't work)

Note that the user's location can usually only be accessed under HTTP_S_.

### Errors

Some possible errors encountered along the setup process and how to resolve them.

#### Incompatible GDAL Setup

"GoLang 1.21" & "gdal-bin 3.4.1+dfsg-1build4 amd64" & "github.com/lukeroth/gdal v0.0.0-20230422025444-05e99f726ff9" don't work together (neither does "github.com/lukeroth/gdal v0.0.0-20230818145556-62d5095a1cda").
lukeroth/gdal [issue #97](https://github.com/lukeroth/gdal/issues/97) suggests to upgrade to gdal 3.6. However, on Ubuntu 22 LTS, this is not an option, as it comes with many dependencies that are only availablei in Debian (as of 2023-11-28).

Observed errors in this case:

```txt
go build -o ./dist/geopki-client ./cmd/geopki-client
# github.com/lukeroth/gdal
../../../../go/pkg/mod/github.com/lukeroth/gdal@v0.0.0-20230422025444-05e99f726ff9/gdal.go:44:11: cannot define new methods on non-local type C.CPLErr
../../../../go/pkg/mod/github.com/lukeroth/gdal@v0.0.0-20230422025444-05e99f726ff9/gdal.go:60:11: cannot define new methods on non-local type C.OGRErr
make: *** [Makefile:4: client] Error 1
```

or:

```bash
# github.com/lukeroth/gdal
../../../../go/pkg/mod/github.com/lukeroth/gdal@v0.0.0-20230818145556-62d5095a1cda/algorithms.go:457:4: unknown field nSizeOfStructure in struct literal of type _Ctype_struct___2
../../../../go/pkg/mod/github.com/lukeroth/gdal@v0.0.0-20230818145556-62d5095a1cda/algorithms.go:475:4: unknown field nSizeOfStructure in struct literal of type _Ctype_struct___1
../../../../go/pkg/mod/github.com/lukeroth/gdal@v0.0.0-20230818145556-62d5095a1cda/algorithms.go:489:4: unknown field nSizeOfStructure in struct literal of type _Ctype_struct___4
../../../../go/pkg/mod/github.com/lukeroth/gdal@v0.0.0-20230818145556-62d5095a1cda/algorithms.go:502:4: unknown field nSizeOfStructure in struct literal of type _Ctype_struct___5
../../../../go/pkg/mod/github.com/lukeroth/gdal@v0.0.0-20230818145556-62d5095a1cda/algorithms.go:515:4: unknown field nSizeOfStructure in struct literal of type _Ctype_struct___0
../../../../go/pkg/mod/github.com/lukeroth/gdal@v0.0.0-20230818145556-62d5095a1cda/algorithms.go:528:4: unknown field nSizeOfStructure in struct literal of type _Ctype_struct___3
```

## Performance Evaluation

This section explains how to reproduce the results from the thesis.
First and foremost, the sampling map produced by `./cmd/db-address-exporter` is required.

The output for the dataset from the thesis (`/osm-dataset/word.parquet` in the [data repository](https://gitlab.inf.ethz.ch/OU-PERRIG/theses/msc_nico_hauser/data/-/tree/main/)) should result in [the sampling map](https://gitlab.inf.ethz.ch/OU-PERRIG/theses/msc_nico_hauser/data/-/blob/main/sampling-maps/sampling-map.csv)`/sampling-maps/sampling-map.csv` in the [data repository](https://gitlab.inf.ethz.ch/OU-PERRIG/theses/msc_nico_hauser/data/-/tree/main/).

The next section explains how this file is used to derive the various sampling maps in `/sampling-maps/`.

### Sampling Maps

There are three types of sampling maps differentiated by their extension.

- `sampling-map.csv` is the output of `./cmd/db-address-exporter`
- `sampling-map-bitstrings-f1.parquet` and `sampling-map-bitstrings-f01.parquet` are generated by `performance/sampling-map-generator.py` and contain for each location in `sampling-map.csv` the pre-computed bit strings for a given relative grid size. The order of rows is shuffeled.
- `sampling-map-bitstrings-f1.json` and `sampling-map-bitstrings-f01.json` are generated by `performance/sampling-map-converter.py` correspond to the parquet version but are in the json format. They are generated by `performance/sampling-map-converter.py`.

The following sections detail the interface of the sampling map scripts.

#### performance/sampling-map-generator.py

Accepts three positional arguments, the path to `sampling-map.csv`, an output path ending in `.parquet` and a float specifying the relative grid size.

**Examples:**

```bash
python3 performance/sampling-map-generator.py /sampling-maps/sampling-map.csv \
                                              /sampling-maps/sampling-map-bitstrings-f1.parquet \
                                              1
```

```bash
python3 performance/sampling-map-generator.py /sampling-maps/sampling-map.csv \
                                              /sampling-maps/sampling-map-bitstrings-f01.parquet \
                                              0.1
```

#### performance/sampling-map-converter.py

Accepts two positional arguments, the path to a `.parquet` sampling map and an output path ending in `.json`.
It converts the given `.parquet` sampling map to a json one.

**Examples:**

```bash
python3 performance/sampling-map-converter.py /sampling-maps/sampling-map-bitstrings-f1.parquet \
                                              /sampling-maps/sampling-map-bitstrings-f1.json
```

```bash
python3 performance/sampling-map-converter.py /sampling-maps/sampling-map-bitstrings-f01.parquet \
                                              /sampling-maps/sampling-map-bitstrings-f01.json
```

### Database Only

To measure the database throughput for the spatial index, prefix matching and integer range queries, the script `database/scripts/performance/performance-evaluation.py` is used.
It accepts two position arguments, a path to a sampling map and an output path.
The output path must end with `.csv`.

The script then requires a set of database parameters:

- `--db-host=127.0.0.1` to specify the database host address
- `--db-port=5432` to specify the database port
- `--db-name=geopki` to specify the database name
- `--db-user=geopki` to specify the database user
- `--db-pass=...` to specify the database password. This parameter is optional and the program asks for user input if it is omited.

With `--repetitions=30`, the number of repetitions for the measurement are specified.
The `--excluding-bit-string-computation` flag decides whether the bit string computation is part of the measured time or not.
By default it is included but for all measurements shown in the thesis it is not.
The parameter `--batch-size=1` specifies the number of queries to be issued per iteration and finally there are flags that determine the test that is run:

- `--postgres-spatial` for the spatial index
- `--postgres-bitstrings-txt` for prefix matching
- `--postgres-bitstrings-int` for integer range queries

To reproduce the measurements from the thesis, use the following commands:

#### Spatial Index

Set up the database using the spatial data as described [here](#alternative).

```bash
python3 database/scripts/performance/performance-evaluation.py \
        ~/sampling-map.csv \
        ~/evaluation/database/spatial.csv \
        --db-host=127.0.0.1 \
        --db-port=5432 \
        --db-name=geopki \
        --db-user=postgres \
        --db-pass="$DATABASE_PASS" \
        --batch-size=1 \
        --repetitions=30 \
        --postgres-spatial
```

#### Prefix Matching

Set up the database using the data as described [here](#import-data).

Then use the following SQL commands to change the format of the database to use a text-based index.
The goal is to add the column `bit_string_51_txt` equal to the text representation of `bit_string_51` and created a clustered index on it.

```sql
ALTER TABLE nodes ADD COLUMN bit_string_51_txt character varying(51) COLLATE "C" NOT NULL GENERATED ALWAYS AS (bit_string_51::character varying(51)) STORED;

CREATE INDEX nodes_bit_string_text_pattern_ops_idx ON nodes(bit_string_51_txt COLLATE "C");
ALTER TABLE IF EXISTS nodes CLUSTER ON nodes_bit_string_text_pattern_ops_idx;
CLUSTER nodes USING nodes_bit_string_text_pattern_ops_idx;
```

Finally, run the following commands:

For `f=1`

```bash
python3 database/scripts/performance/performance-evaluation.py \
        /sampling-maps//sampling-map-bitstrings-f1.parquet \
        /evaluation/database/bitstrings-txt-f1.csv \
        --db-host=127.0.0.1 \
        --db-port=5432 \
        --db-name=geopki \
        --db-user=postgres \
        --db-pass="$DATABASE_PASS" \
        --batch-size=1 \
        --excluding-bit-string-computation \
        --repetitions=30 \
        --postgres-bitstrings-txt
```

For `f=0.1`

```bash
python3 database/scripts/performance/performance-evaluation.py \
        /sampling-maps/sampling-map-bitstrings-f01.parquet \
        /evaluation/database/bitstrings-txt-f01.csv \
        --db-host=127.0.0.1 \
        --db-port=5432 \
        --db-name=geopki \
        --db-user=postgres \
        --db-pass="$DATABASE_PASS" \
        --batch-size=1 \
        --excluding-bit-string-computation \
        --repetitions=30 \
        --postgres-bitstrings-txt
```

#### Integer Range Query

Set up the database using the data as described [here](#import-data).

For `f=1`

```bash
python3 database/scripts/performance/performance-evaluation.py \
        /sampling-maps/sampling-map-bitstrings-f1.parquet \
        /evaluation/database/bitstrings-int-f1.csv \
        --db-host=127.0.0.1 \
        --db-port=5432 \
        --db-name=geopki \
        --db-user=postgres \
        --db-pass="$DATABASE_PASS" \
        --batch-size=1 \
        --excluding-bit-string-computation \
        --repetitions=30 \
        --postgres-bitstrings-int
```

For `f=0.1`

```bash
python3 database/scripts/performance/performance-evaluation.py \
        /sampling-maps/sampling-map-bitstrings-f01.parquet \
        /evaluation/database/bitstrings-int-f01.csv \
        --db-host=127.0.0.1 \
        --db-port=5432 \
        --db-name=geopki \
        --db-user=postgres \
        --db-pass="$DATABASE_PASS" \
        --batch-size=1 \
        --excluding-bit-string-computation \
        --repetitions=30 \
        --postgres-bitstrings-int
```

#### Depths

There are two depth measurements we performed.
The first is to measure the depth of the locations where certificates are placed and the second, the depth of sparse SMT nodes.

##### Certificate Depths

To collect statistics on the maximum depth of locations, the script `./database/scripts/depth/smt-depth-analysis.py` is used.
It accepts one positional argument, the output path.

The script then requires a set of database parameters:

- `--db-host=127.0.0.1` to specify the database host address
- `--db-port=5432` to specify the database port
- `--db-name=geopki` to specify the database name
- `--db-user=geopki` to specify the database user
- `--db-pass=...` to specify the database password. This parameter is optional and the program asks for user input if it is omited.

Finally, the script requires the `--sampling-map` parameter which specifies the locations to query.
This parameter should point to the `.csv` sampling map.

Our measurements can be found in `./database/measurements/depth.csv.zip`.

**Example:**

```bash
python3 database/scripts/depth/smt-depth-analysis.py \
        ./database/measurements/depth.csv \
        --sampling-map=/sampling-maps/sampling-map.csv \
        --db-host=127.0.0.1 \
        --db-port=5432 \
        --db-name=geopki \
        --db-user=postgres \
        --db-pass="$DATABASE_PASS"
```

##### Sparse SMT Node Depths

To collect statistics on sparse SMT nodes, the SQL query in `./database/scripts/depth/leaves.sql` is used.
You can e.g. use pgAdmin4 to run the query and store the csv result at `./database/measurements/leaves.csv`.
Our measurements can be found in `./database/measurements/leaves.csv.zip`.

#### Altitude Distribution

The altitude distribution of certificates is directly written by `./cmd/db-address-exporter`.
Our measurements can be found in `./database/measurements/altitude.csv.zip`.

#### Plotting the Results

To plot the results use `python3 database/measurements/plot.py`.
The script assumes the following files to be in the same directory as `plot.py`:

- `performance-evaluation-spatial.csv` (Spatial Index)
- `performance-evaluation-bitstring-txt-f1.csv` (Prefix Matching)
- `performance-evaluation-bitstring-int-f1.csv` (Integer Range Queries)
- `leaves.csv` (Sparse SMT Node Depths, contained in `./database/measurements/leaves.csv.zip`)
- `depth.csv` (Certificate Depths, contained in `./database/measurements/depth.csv.zip`)
- `altitude.csv` (Altitude SMT Node Depths, contained in `./database/measurements/altitude.csv.zip`)

#### Plotting Certificate Size Distribution

To plot `cert-sizes.csv` of `./cmd/db-address-exporter` (contained in `./performance/cert-sizes/cert-sizes-measurements.zip`), the script `./performance/cert-sizes/cert-sizes.py` is used.

It accepts two position arguments, the path to `cert-sizes.csv` and an output directory.
It then plots the data and writes the images to the output directory.

**Example:**

```bash
python3 performance/cert-sizes/cert-sizes.py performance/cert-sizes/cert-sizes.csv performance/cert-sizes
```

### Full System

This section detauls how to measure the performance of the full system.
All scripts require a running geopki server.
[This section](#geopki-server) describes how to run an instance.

After an instance is running, three different types of measurements can be performed.
They are detailed in the following sections.

#### Metrics on Individual Requests and Responses

To obtain metrics on individual requests and responses, queries are sequentially sent to the server.
The modified geopki-client `./cmd/geopki-client-performance` is used.
Compared to the [CLI version](#geopki-client), it collects the relevant metrics and prints them to the console.

For measurements, the python script `performance/client/client-performance.py` is used.
It accepts three positional arguments, the path to `sampling-map.csv`, an output path ending in `.csv` and the Base64 encoded public key of the server.

The Base64 encoded public key can be obtained using the helper utilitty `./cmd/geopki-public-key`:

```bash
go run ./cmd/geopki-public-key --address=http://127.0.0.1:1234
```

On top of the positional parameters, the script accepts the parameter `--address=http://127.0.0.1:1234` to specify the server's address as well as `--locations=1000` to specify the number of queries.

To reproduce the measurements from the thesis, use the following parameters:

For `f=1`

```bash
make clean
make client-performance

python3 performance/client/client-performance.py \
        /sampling-maps/sampling-map.csv \
        /evaluation/client-performance-f1.csv \
        "$PUBLIC_KEY" \
        --address=http://127.0.0.1:1234 \
        --locations=100000
```

For `f=0.1`, manually modify `cmd/geopki-client-performance/geopki-client-performance.go` and set `F_GROW=0.1` and re-build the binary using `make`.

```bash
make clean
make client-performance

python3 performance/client/client-performance.py \
        /sampling-maps/sampling-map.csv \
        /evaluation/client-performance-f01.csv \
        "$PUBLIC_KEY" \
        --address=http://127.0.0.1:1234 \
        --locations=100000
```

##### Plotting the Results

To plot the results, use `performance/client/client-performance-plot.py`.
The script accepts two positional arguments, the output of `performance/client/client-performance.py` and an output folder.
It plots the measurement and writes the images to the given output folder.

The thesis measurements are contained in `./performance/client/client-performance-measurements.zip`.

**Example:**

```bash
python3 performance/client/client-performance-plot.py performance/client/client-performance.csv performance/client
```

#### Throughput

To measure the throughput the server can handle the specialized client `./cmd/geopki-client-throughput` is used.
It spawns the given number of goroutines and sends pre-computed queries to the server during the specified time.
Bit string computations are all performed beforehand (by using the pre-computed sampling maps) and the cryptographic response verification is not performed.

The program writes the number of successful and failed responses to the console.
Moreover it also collects the latencies for the issued queries and writes them to the console as well.
Compared to the [previous section](#metrics-on-individual-requests-and-responses) where all requests are performed sequentially, this measures the latency during high load.

For measurements, the python script `./performance/throughput/throughput.py` is used.
It accepts two positional arguments, the path to a `.json` sampling map and an output path ending in `.csv`.

Moreover, the script accepts the parameter `--address=http://127.0.0.1:1234` to specify the server's address as well as `--repetitions=30` to specify how often the measurement should be repeated.

To reproduce the measurements from the thesis, use the following parameters:

For `f=1`

```bash
make clean
make client-throughput

python3 performance/throughput/throughput.py \
        /sampling-maps/sampling-map-bitstrings-f1.json \
        /evaluation/throughput-f1.csv \
        --address=http://127.0.0.1:1234 \
        --repetitions=30
```

For `f=0.1`

```bash
python3 performance/throughput/throughput.py \
        /sampling-maps/sampling-map-bitstrings-f01.json \
        /evaluation/throughput-f01.csv \
        --address=http://127.0.0.1:1234 \
        --repetitions=30
```

##### Plotting the Results

To plot the results, use `performance/throughput/throughput-plot.py`.
The script accepts two positional arguments, the output of `performance/throughput/throughput.py` and an output folder.
It plots the measurement and writes the images to the given output folder.

The thesis measurements are contained in `./performance/throughput/throughput-measurements.zip`.

**Example:**

```bash
python3 performance/throughput/throughput-plot.py performance/throughput/throughput.csv performance/throughput
```

#### Ingestion

For ingestion, the specialized client `./cmd/geopki-client-ingestion` is used.
Compared to all other clients, this one does not query the server but instead access the ingestion endpoints.
The program simply measures the time it takes to insert the certificates passed to it.

For measurements, the python script `./performance/ingestion/ingestion.py` is used.
It accepts two positional arguments, the path to a website density map, e.g. `europe-10.parquet`, and an output path ending in `.csv`.

Moreover, the script accepts the parameter `--address=http://127.0.0.1:1234` to specify the server's address, `--insertion-key=abc` to specify the insertion key, as well as `--repetitions=30` to specify how often the measurement should be repeated.

Finally, the script accepts an optional `--with-altitude` flag to determine whether the generated certificates should be placed in the altitude subtrees (if the flag is set) or just the surface tree (if the flag is not set).

The script randomly samples locations from the given density map, weighted by the number of websites in the respective grid cell.
Within the grid cell, the script then samples a point uniformly at random and constructs a square at this location with a side-length of 30m.
If the `--with-altitude` flag is set, a random altitude is drawn uniformly at random to allow a height that is also drawn uniformly at random from the range [3m, 30m].

The script measures different batch sizes, i.e. different numbers of certificates that are sent to the server at the same time.

To reproduce the measurements from the thesis, use the following parameters:

```bash
make clean
make client-ingestion

python3 performance/ingestion/ingestion.py \
        /website-density/europe-10.parquet \
        /evaluation/ingestion.csv \
        --address=http://127.0.0.1:1234 \
        --insertion-key=abc \
        --repetitions=30 \
        --with-altitude
```

Note that the ingestion only modifies the `nodes_next` table.
To swap the `nodes` and `nodes_next` table, a release must be issued.

The last section details how to do this.

##### Plotting the Results

To plot the results, use `performance/ingestion/ingestion-plot.py`.
The script accepts two positional arguments, the output of `performance/ingestion/ingestion.py` and an output folder.
It plots the measurement and writes the images to the given output folder.

The thesis measurements are contained in `./performance/ingestion/ingestion.csv`.

**Example:**

```bash
python3 performance/ingestion/ingestion-plot.py performance/ingestion/ingestion.csv performance/ingestion
```

##### Release

To issue a release, the specialized client `./cmd/release` is used.
It accesses a special endpoint to trigger a release.

The timing measurements of how long the release takes is written to the server's console.

**Example:**

```bash
go run ./cmd/release --address=http://127.0.0.1:1234 --insertion-key=abc
```
