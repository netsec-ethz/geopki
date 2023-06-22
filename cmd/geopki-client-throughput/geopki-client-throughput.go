package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"geopki/pkg/comm"
	"geopki/pkg/geometry"
)

const (
	F_GROW = 0.1
)

type Query struct {
	Longitude float64 `json:"longitude"`
	Latitude  float64 `json:"latitude"`
	Altitude  float64 `json:"altitude"`
	Radius    uint64  `json:"radius"`
}

type Results struct {
	successfulRequests int
	failedRequests     int
}

func main() {

	var address string
	var queriesJson string
	var runningTime float64
	var includeCertificates bool

	flag.StringVar(&address, "address", "", "The HTTP address of the server to send the request to")
	flag.StringVar(&queriesJson, "queries", `[{"longitude":8.5470994,"latitude":47.3762348,"altitude":0,"radius":10}]`, "The json encoded queries")
	flag.Float64Var(&runningTime, "time", 8, "The number of seconds the program should run")
	flag.Parse()

	var querySet []*Query
	err := json.Unmarshal([]byte(queriesJson), &querySet)
	if err != nil {
		fmt.Fprintf(os.Stderr, "received invalid query set: %v\n", err)
		os.Exit(1)
	}

	queries := make([]*comm.Query, len(querySet))
	for i, q := range querySet {
		query, err := comm.NewQuery(q.Longitude, q.Latitude, q.Altitude, q.Radius, F_GROW, &geometry.GdalCircleApproximator{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed building a query using (%f,%f,%f,%d)\n", q.Longitude, q.Latitude, q.Altitude, q.Radius)
			os.Exit(1)
		}

		queries[i] = query
	}

	numQueries := len(queries)
	quit := make(chan bool)
	results := make(chan Results)

	now := time.Now()
	sleepDuration := time.Duration(runningTime * float64(time.Second))
	stopTime := now.Add(sleepDuration)

	// start querying
	go func() {
		// keep track of the current query index
		i := 0

		successfulRequests := 0
		failedRequests := 0

		for {
			select {
			case <-quit:
				// when quit signal is received, return results to main thread
				results <- Results{
					successfulRequests: successfulRequests,
					failedRequests:     failedRequests,
				}

				// and stop quering
				return
			default:
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
				// fmt.Printf("received response: %v\n", requests[len(requests)-1])

				// use a different query next time
				i = (i + 1) % numQueries
			}
		}
	}()

	// sleep in main thread
	time.Sleep(sleepDuration)

	// stop go routine
	quit <- true
	// receive results from go routine
	res := <-results

	fmt.Printf(
		"%f,%d,%d\n",
		runningTime,
		res.successfulRequests,
		res.failedRequests,
	)
}
