package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"geopki/pkg/bitstring"
	"geopki/pkg/comm"
)

const (
	MAX_QUERIES_PER_SECOND = 1000
)

type Query struct {
	BitStrings []string `json:"bit_strings"`
}

type Results struct {
	successfulRequests int
	failedRequests     int
	latencies          []time.Duration
}

// mutex controlling whether the threads are running
var runningMutex sync.RWMutex
var stopTime time.Time

func measureThroughput(
	querySet []*Query,
	address string,
	includeCertificates bool,
	results chan Results,
	readyMutex *sync.Mutex,
) {
	numQueries := len(querySet)

	queries := make([]*comm.Query, numQueries)
	for i := range queries {
		q := querySet[i]

		bitStrings := make([]bitstring.RawXYBitString, len(q.BitStrings))
		for i, bitString := range q.BitStrings {
			b, err := strconv.ParseUint(bitString, 2, 51)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed parsing bit string '%s'\n", bitString)
				os.Exit(1)
			}

			bitStrings[i] = bitstring.RawXYBitString{
				XYBitString:    b << (64 - len(bitString)),
				XYBitStringLen: uint8(len(bitString)),
			}
		}

		// set altitude to 0
		query := comm.Query{
			XYBitStrings: bitStrings,
			MinAltitude:  0,
			MaxAltitude:  int16(bitstring.C_Z),
		}

		queries[i] = &query
	}

	// keep track of the current query index
	i := 0

	successfulRequests := 0
	failedRequests := 0
	latencies := make([]time.Duration, 0, numQueries)

	// signal that we're ready
	readyMutex.Unlock()

	// acquire mutex preventing the main goroutine from terminating
	runningMutex.RLock()

	for {
		if time.Now().After(stopTime) {
			// when time is over, return results to main thread
			results <- Results{
				successfulRequests: successfulRequests,
				failedRequests:     failedRequests,
				latencies:          latencies,
			}

			// release running mutex
			runningMutex.RUnlock()

			// and stop the go routine
			return
		}

		before := time.Now()

		// otherwise send query
		_, _, _, err := comm.QueryMapServer(
			address,
			queries[i],
			includeCertificates,
		)

		now := time.Now()
		latencies = append(latencies, now.Sub(before))

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
	if queryCount > len(querySet) {
		fmt.Fprintf(os.Stderr, "provided query set is too small, %d required, %d given\n", queryCount, len(querySet))
		os.Exit(1)
	}

	results := make(chan Results, threads)

	// acquire write lock blocking any thread from starting
	runningMutex.Lock()

	queriesPerThread := len(querySet) / threads
	readyMutexes := make([]*sync.Mutex, threads)

	// initialize go routines, divide queries slice
	for i := 0; i < threads; i++ {
		// initialize lock in locked state
		var readyMutex sync.Mutex
		readyMutex.Lock()

		readyMutexes[i] = &readyMutex

		go measureThroughput(
			querySet[i*queriesPerThread:(i+1)*queriesPerThread],
			address,
			includeCertificates,
			results,
			&readyMutex,
		)
	}

	// wait for all routines to become ready
	for i := 0; i < threads; i++ {
		readyMutexes[i].Lock()
	}

	// once all are ready, define the start & stop time
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
	var latencies []time.Duration

CollectResults:
	for {
		select {
		// receive results from go routine
		case res := <-results:
			successfulRequests += res.successfulRequests
			failedRequests += res.failedRequests
			latencies = append(latencies, res.latencies...)
			resultCount++
		default:
			// if no results are available anymore, stop
			break CollectResults
		}
	}

	if resultCount != threads {
		log.Fatalf("Spawned %d threads but received %d results?!\n", threads, resultCount)
	}

	stringLatencies := make([]string, len(latencies))
	for i, latency := range latencies {
		stringLatencies[i] = fmt.Sprintf("%f", latency.Seconds())
	}

	fmt.Printf(
		"%d,%d,%t,%d,%d,[%s]\n",
		threads,
		runningTime,
		includeCertificates,
		successfulRequests,
		failedRequests,
		strings.Join(stringLatencies, ","),
	)
}
