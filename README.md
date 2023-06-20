# geopki

Prototype implementation for a PKI extension that allows the querying of the set of all certificates associated with a given geographical area.
This is analogous to [F-PKI](https://netsec.ethz.ch/publications/papers/chuat2022fpki.pdf) which provides the same functionality for domains, i.e. allows the querying of the set of all certificates for a given domain.

## Table of Contents

- [geopki](#geopki)
  - [Table of Contents](#table-of-contents)
  - [Project Structure](#project-structure)
    - [Go Server \& Client](#go-server--client)
    - [Performance Measurements](#performance-measurements)
    - [Database Input](#database-input)
    - [Database Performance Measurements](#database-performance-measurements)
  - [Setup](#setup)
    - [Prerequisites](#prerequisites)
      - [Go](#go)
      - [Python](#python)
    - [GeoPKI Server](#geopki-server)
    - [GeoPKI Client](#geopki-client)


## Project Structure

### Go Server & Client
Go Files, relevant for the server and client implementation
```
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
|   |   └── z-subtrees                    # set of functions for a scheme with z subtrees
|   |       └── ...
|   |
|   └── tables                         # SQL tables schemas
|       ├── certificates.sql               # the table schema for the certificates table
|       └── nodes.sql                      # the table schema for the nodes table
|
├── pkg                           # code potenially re-usable for projects building on this one
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
|       └── verification.go               # code for verifying server responses client-side
|
├── proto                         # protobuf messages
|   ├── request.proto                 # protobuf message for a query request sent by a client
|   └── response.proto                # protobuf message for a query response sent by a server
|
├── go.mod                        # standard go dependency managment file
└── go.sum                        # standard go dependency managment file
```

### Performance Measurements

```
.
├── cmd                           # the go programs
|   └── bitstring-performance         # program for measuring the performance of the bit string computations given a volume
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

```
database/scripts/input
├── db-address-exporter.py        # generates SQL INSERT files based the output of the 'location-correlation'
├── db-input-analyzer.py          # plots the number of bit strings used for DBs with and without z subtrees
├── db-sanity-check-generator.py  # generates sql files for a dummy table to verify the measurements
└── graph-db-address-exporter.py  # generates CSV files that can be imported into neo4j 

```

### Database Performance Measurements

```
database/scripts/performance
├── db-types                      # the scripts for running performance tests on various schemes
|   ├── neo4j-run.py                              # performance test on neo4j
|   ├── postgres-baseline-run.py                  # performance test on postgres, baseline table
|   ├── postgres-bitstring-run.py                 # performance test on postgres using bitstrings
|   ├── postgres-bitstring-int-run.py             # performance test on postgres using integer bitstrings
|   ├── postgres-bitstring-int-z-subtrees-run.py  # performance test on postgres using integer and z subtrees
|   └── postgres-spatial-run.py                   # performance test on postgres using spatial index
|
└── performance-evaluation.py     # runs one script in db-types several times and collects the results

```

## Setup

### Prerequisites

#### Go
For running the geopki client or server, go is required.
Most dependencies are automatically downloaded and managed by go.
The only exception is [Geospatial Data Abstraction Library (GDAL)](https://gdal.org/) which has to be [installed seperately](https://gdal.org/download.html).
[Go bindings](https://github.com/lukeroth/gdal) are used to interact with GDAL unless with the exception of the wasm client that uses [S2](https://github.com/golang/geo).

#### Python
For running the python scripts, python and the relevant dependencies have to be installed.
Note that python is **not** required to run the geopki client nor the server but scripts such as the performance measurements are written in python.

1. Install `python3` (tested on version `3.11.3`), should probably work on later versions too
2. Install pip dependencies using `pip3 install -r requirements.txt`

### GeoPKI Server

1. Setup the database
   1. Setup a postgres instance
   2. Create a new database
   3. Create the required tables
      1. Run the code in `database/tables/certificates.sql` to create the `certificates` table
      2. Run the code in `database/tables/nodes.sql` to create the `nodes` table
   4. Define the required functions
      1. Define functions for computing the difference, intersection and union of arrays
         1. Run the code in `database/functions/z-subtrees/array_difference.sql`
         2. Run the code in `database/functions/z-subtrees/array_intersect.sql`
         3. Run the code in `database/functions/z-subtrees/array_union.sql`
      2. Define function for updating the hash of a node after ingestion of new data / deletion of expired data
         1. Run the code in `database/functions/z-subtrees/update_children_hashes.sql`
2. Setup a [trillian](https://github.com/google/trillian) instance
   1. Follow the instructions on the trillian repo: https://github.com/google/trillian/tree/v1.5.2/examples/deployment
   2. The [local deployment with docker](https://github.com/google/trillian/tree/v1.5.2/examples/deployment#local-deployments) is straight forward
   3. Create a new tree using `go run ./cmd/createtree --admin_server=localhost:8090`
   4. Copy the log id from the output
      1. The output should contain a line like `Initialised Log (2947592571015490951) with new SignedTreeHead` where `2947592571015490951` corresponds to the log id.
3. Define environment variables
   1. `DATABASE_URL`
      1. Set the database url for the postgres database using `export DATABASE_URL=postgresql://<username>:<password>@localhost:5432/<postgres database>` and replacing the `<>` with values set when setting up the postgres database.
   2. `PRIVATE_KEY`
      1. Set the private key of the geopki server that is used to sign the map and consistency heads using `export PRIVATE_KEY=MHcCAQEEIGv4NvMEZL3JjuQ8BnWVkTwkwCtXZhpkozMu1iCUvXBdoAoGCCqGSM49AwEHoUQDQgAEvvlGuoiglGCYNXJ0rpbKwQuXIQnIHE2mCDrDtlm7KxF4//6w3quLK/4Q8DwkM27zkOpnjv701tdFuBbf5EloqA==`
      2. The format is base64 encoded bytes that are accepted by go's `ParseECPrivateKey` function which according to the documentation means any EC private key in SEC 1, ASN.1 DER form.
      3. The easiest way to obtain a correctly formatted value is to run the server once without having the value set. The server will then print a randomly generated key to the console.
   3. `CERT_INSERT_KEY`
      1. The server exposes the endpoint `/v1/insert` for showcasing the ingestion and deletion functionality. Since this endpoint causes a lot of computation and allows the insertion of arbitrary data at the moment, it is protected by a secret. In the future no such endpoint should exist and the map server should on its own crawl the CT logs.
      2. Use `export CERT_INSERT_KEY=xxxxxxxxx` to set that value.
4. Run the server instance using `go run ./cmd/geopki-server --address=0.0.0.0 --port=1234 --trillian-address=localhost:8090 --clog-id=<trillian log id>`.
   1. All arguments except `clog-id` can be omitted if the just shown default values should be used.
   2. If you forgot the log id, you can use `cmd/list-trees` (in the geopki repo) to list all trees: `go run ./cmd/list-trees --admin_server=localhost:8090`

### GeoPKI Client

The client program can be run using `go run ./cmd/geopki-client --address=http://localhost:1234 --longitude=<lon> --latitude=<lat> --altitude=<alt> --radius=<rad> --include-certificates`.
When run without the `--include-certificates` flag, only the certificate hashes are obtained.