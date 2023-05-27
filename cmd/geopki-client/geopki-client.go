package main

import (
	"flag"
	"geopki/pkg/comm"
	"log"
	"math"
)

const (
	F_GROW = 1
)

func main() {
	var address string
	var longitude, latitude, altitude float64
	var radius uint64

	flag.StringVar(&address, "address", "", "The HTTP adress of the server to send the request to")
	flag.Float64Var(&longitude, "longitude", 190, "The longitude to query for")
	flag.Float64Var(&latitude, "latitude", 100, "The latitude to query for")
	flag.Float64Var(&altitude, "altitude", math.Inf(0), "The altitude to query for")
	flag.Uint64Var(&radius, "radius", 10, "The radius for the query in meters")
	flag.Parse()

	response, err := comm.Query(
		address,
		longitude,
		latitude,
		altitude,
		radius,
		F_GROW,
	)
	if err != nil {
		log.Fatalf("request failed: %v\n", err)
	}

	println(response)
}
