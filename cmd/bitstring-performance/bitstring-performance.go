package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"time"

	"geopki/pkg/comm"
)

func main() {
	var longitude, latitude, altitude float64
	var radius uint64
	var fGrow float64
	var repetitions int

	flag.Float64Var(&longitude, "longitude", 190, "The longitude to query for")
	flag.Float64Var(&latitude, "latitude", 100, "The latitude to query for")
	flag.Float64Var(&altitude, "altitude", math.Inf(0), "The altitude to query for")
	flag.Uint64Var(&radius, "radius", 10, "The radius for the query in meters")
	flag.Float64Var(&fGrow, "f", 1, "The grow factor f")
	flag.IntVar(&repetitions, "repetitions", 30, "The number of repetitions for this measurement")
	flag.Parse()

	for i := 0; i < repetitions; i++ {
		var start time.Time
		var duration time.Duration

		start = time.Now()
		query, err := comm.NewQuery(longitude, latitude, altitude, radius, fGrow)
		duration = time.Since(start)

		if err != nil {
			log.Fatalf("❌ building query: %v\n", err)
		}

		fmt.Printf("%f,%d,%f,%d\n", fGrow, radius, duration.Seconds(), len(query.XYBitStrings))
	}
}
