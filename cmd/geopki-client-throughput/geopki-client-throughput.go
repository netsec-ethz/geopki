package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"geopki/pkg/bitstring"
	"geopki/pkg/comm"
	"geopki/pkg/geometry"
)

const (
	F_GROW                 = 0.1
	MAX_QUERIES_PER_SECOND = 50
)

type Query struct {
	Longitude float64 `json:"longitude"`
	Latitude  float64 `json:"latitude"`
	Radius    uint64  `json:"radius"`
}

type Results struct {
	successfulRequests int
	failedRequests     int
}

// mutex controlling whether the threads are running
var runningMutex sync.RWMutex
var stopTime time.Time

func measureThroughput(
	queries []*comm.Query,
	address string,
	includeCertificates bool,
	results chan Results,
) {
	// keep track of the current query index
	numQueries := len(queries)
	i := 0

	successfulRequests := 0
	failedRequests := 0

	// acquire mutex preventing the main goroutine from terminating
	runningMutex.RLock()

	for {
		if time.Now().After(stopTime) {
			// when time is over, return results to main thread
			results <- Results{
				successfulRequests: successfulRequests,
				failedRequests:     failedRequests,
			}

			// release running mutex
			runningMutex.RUnlock()

			// and stop the go routine
			return
		}

		// otherwise send query
		_, _, _, err := comm.QueryMapServer(
			address,
			queries[i],
			includeCertificates,
		)

		now := time.Now()

		if err != nil {
			// write error to stderr but continue, might be to overloaded server
			fmt.Fprintf(os.Stderr, "request failed: %v\n", err)

			if now.Before(stopTime) {
				failedRequests++
			}
		} else if now.Before(stopTime) {
			successfulRequests++
		}

		// use a different query next time
		i = (i + 1) % numQueries
	}
}

func main() {

	var address string
	var queriesInput string
	var queriesJson string
	var runningTime int
	var threads int
	var includeCertificates bool

	flag.StringVar(&address, "address", "", "The HTTP address of the server to send the request to")
	flag.StringVar(&queriesInput, "queries", `[{"longitude":8.5470994,"latitude":47.3762348,"altitude":0,"radius":10}]`, "The json encoded queries")
	flag.IntVar(&runningTime, "time", 8, "The number of seconds the program should run")
	flag.IntVar(&threads, "threads", 1, "The number of threads the program should run")
	flag.BoolVar(&includeCertificates, "include-certificates", false, "Whether to include the certificates")
	flag.Parse()

	var querySet []*Query

	if _, err := os.Stat(queriesInput); err == nil {
		// read json from disk
		content, err := os.ReadFile(queriesInput)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed reading file '%s': %v\n", queriesInput, err)
			os.Exit(1)
		}

		queriesJson = string(content)
	} else {
		queriesJson = queriesInput
	}

	err := json.Unmarshal([]byte(queriesJson), &querySet)
	if err != nil {
		fmt.Fprintf(os.Stderr, "received invalid query set '%s': %v\n", queriesInput, err)
		os.Exit(1)
	}

	queryCount := threads * int(runningTime) * MAX_QUERIES_PER_SECOND
	if queryCount >= len(querySet) {
		fmt.Fprintf(os.Stderr, "provided query set is too small, %d required, %d given\n", queryCount, len(querySet))
		os.Exit(1)
	}

	queries := make([]*comm.Query, queryCount)
	for i := range queries {
		q := querySet[i]

		// set altitude to 0
		query, err := comm.NewQuery(q.Longitude, q.Latitude, 0, q.Radius, F_GROW, &geometry.GdalCircleApproximator{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed building a query using (%f,%f,%f,%d)\n", q.Longitude, q.Latitude, 0.0, q.Radius)
			os.Exit(1)
		}
		// and then overwrite min and max altitude to cover the full altitude range
		query.MinAltitude = 0
		query.MaxAltitude = int16(bitstring.C_Z)

		queries[i] = query
	}

	results := make(chan Results, threads)

	// acquire write lock blocking any thread from starting
	runningMutex.Lock()

	if len(queries)%threads != 0 {
		log.Fatalf("# of queries (%d) is not a multiple of # of threads (%d)\n", len(queries), threads)
	}
	queriesPerThread := len(queries) / threads

	// initialize go routines, divide queries slice
	for i := 0; i < threads; i++ {
		go measureThroughput(
			queries[i*queriesPerThread:(i+1)*queriesPerThread],
			address,
			includeCertificates,
			results,
		)
	}

	now := time.Now()

	// set start time to now
	startTime := now
	// compute stop time
	stopTime = startTime.Add(time.Duration(float64(runningTime) * float64(time.Second)))

	// release lock starting the threads
	runningMutex.Unlock()
	// sleep a second ensuring a context switch s.t. the query threads acquire a read lock
	time.Sleep(time.Second)
	// re-acquire write lock, will block until all threads finished and released their read lock
	runningMutex.Lock()

	// collect results = everything in the results channel at the moment
	successfulRequests := 0
	failedRequests := 0
	resultCount := 0

CollectResults:
	for {
		select {
		// receive results from go routine
		case res := <-results:
			successfulRequests += res.successfulRequests
			failedRequests += res.failedRequests
			resultCount++
		default:
			// if no results are available anymore, stop
			break CollectResults
		}
	}

	if resultCount != threads {
		log.Fatalf("Spawned %d threads but received %d results?!\n", threads, resultCount)
	}

	fmt.Printf(
		"%d,%d,%t,%d,%d\n",
		threads,
		runningTime,
		includeCertificates,
		successfulRequests,
		failedRequests,
	)
}
