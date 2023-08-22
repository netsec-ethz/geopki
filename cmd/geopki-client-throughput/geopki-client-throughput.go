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
	// santity check: assumed number of requests per thread per second. the input dataset needs to be greater than this value
	MAX_QUERIES_PER_SECOND = 1000
)

// data type for the input query
type Query struct {
	// the query surface bit strings
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

// go-routine that measures the throughput for a given query set
func measureThroughput(
	// the function iterates over the query set and sends queries from this set sequentially
	querySet []*Query,
	// the map server's address
	address string,
	// whether the query should also request the certificate payloads or just the hashes
	includeCertificates bool,
	// channel for sending the results
	results chan Results,
	// mutex indicating the go-routine is ready to execute
	readyMutex *sync.Mutex,
) {
	numQueries := len(querySet)

	// transform the input queries to comm.Query
	queries := make([]*comm.Query, numQueries)
	for i := range queries {
		q := querySet[i]

		// transform the bit strings of type 'string' to 'RawXYBitString'
		bitStrings := make([]bitstring.RawXYBitString, len(q.BitStrings))
		for i, bitString := range q.BitStrings {
			// parse the bit string as a base-2, 51 bit number
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

		// query the whole altitude range, i.e. use the minimum and maximum values
		query := comm.Query{
			XYBitStrings: bitStrings,
			MinAltitude:  0,
			MaxAltitude:  int16(bitstring.C_Z),
		}

		queries[i] = &query
	}

	// keep track of the current query index
	i := 0

	// initialize the result values
	successfulRequests := 0
	failedRequests := 0
	latencies := make([]time.Duration, 0, numQueries)

	// signal that we're ready
	readyMutex.Unlock()

	// acquire (shared) mutex preventing the main goroutine from terminating
	runningMutex.RLock()

	// loop indefinitely
	for {
		// until the current is after the specified stop time
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

		// measure query latency
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
			// only increase 'successfulRequests' if the time is still before 'stopTime'
			successfulRequests++
		}

		// use a different query next time
		i = (i + 1) % numQueries
	}
}

func main() {

	// CLI arguments described by the help messages blow
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

	// parse the input query set
	var querySet []*Query

	// check if 'queriesInput' refers to a file
	if _, err := os.Stat(queriesInput); err == nil {
		// if it does, read the json data from the disk
		content, err := os.ReadFile(queriesInput)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed reading file '%s': %v\n", queriesInput, err)
			os.Exit(1)
		}

		queriesJson = string(content)
	} else {
		// if not, interpret the 'queriesInput' as json directly
		queriesJson = queriesInput
	}

	// unmarshal the query set
	err := json.Unmarshal([]byte(queriesJson), &querySet)
	if err != nil {
		fmt.Fprintf(os.Stderr, "received invalid query set '%s': %v\n", queriesInput, err)
		os.Exit(1)
	}

	// ensure enough queries are supplied to be able to only issue unique queries
	// under the assumption each thread can issue at most MAX_QUERIES_PER_SECOND queries per second
	queryCount := threads * int(runningTime) * MAX_QUERIES_PER_SECOND
	if queryCount > len(querySet) {
		fmt.Fprintf(os.Stderr, "provided query set is too small, %d required, %d given\n", queryCount, len(querySet))
		os.Exit(1)
	}

	// prepare a channel for the results
	results := make(chan Results, threads)

	// acquire write lock blocking any thread from starting
	runningMutex.Lock()

	// divide the query set among the threads
	queriesPerThread := len(querySet) / threads

	// create list of ready mutexes indicating whether a thread is ready
	readyMutexes := make([]*sync.Mutex, threads)

	// initialize go routines, divide queries slice
	for i := 0; i < threads; i++ {
		// initialize lock in locked state
		var readyMutex sync.Mutex
		readyMutex.Lock()

		// store mutex in array
		readyMutexes[i] = &readyMutex

		// initialize go-routine. when they are ready they will unlock their own 'readyMutex'
		go measureThroughput(
			// divide the query set among the threads
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

	// release 'runningMutex' to start all go-rotuines in parallel
	runningMutex.Unlock()

	// sleep a second to ensure a context switch s.t. the query threads acquire a read lock
	time.Sleep(time.Second)

	// re-acquire write lock, will block until all threads finished and released their read lock
	runningMutex.Lock()

	// collect results, i.e. everything in the results channel at the moment
	successfulRequests := 0
	failedRequests := 0
	resultCount := 0
	var latencies []time.Duration

CollectResults:
	for {
		select {
		// receive results from go routine
		case res := <-results:
			// increase request counters,
			successfulRequests += res.successfulRequests
			failedRequests += res.failedRequests

			// append latencies
			latencies = append(latencies, res.latencies...)

			// and increase result counter
			resultCount++
		default:
			// if no results are available anymore, stop
			break CollectResults
		}
	}

	// sanity check to ensure we received all results
	if resultCount != threads {
		log.Fatalf("Spawned %d threads but received %d results?!\n", threads, resultCount)
	}

	// format the measured latency values
	stringLatencies := make([]string, len(latencies))
	for i, latency := range latencies {
		stringLatencies[i] = fmt.Sprintf("%f", latency.Seconds())
	}

	fmt.Printf(
		"%d,%d,%t,%d,%d,\"[%s]\"\n",
		threads,
		runningTime,
		includeCertificates,
		successfulRequests,
		failedRequests,
		strings.Join(stringLatencies, ","),
	)
}
