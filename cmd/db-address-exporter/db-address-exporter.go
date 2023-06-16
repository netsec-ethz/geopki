package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/segmentio/parquet-go"
)

type Coordinate struct {
	Longitude float64 `parquet:"lon"`
	Latitude  float64 `parquet:"lat"`
}
type Polygon []Coordinate
type MultiPolygon []Polygon

type RowType struct {
	Domain        string `parquet:"domain"`
	CertificateId string `parquet:"certificate_id"`
	// ListOfMultipolygons []MultiPolygon `parquet:"list_of_multipolygons,list"`
	ListOfLevels     []string `parquet:"list_of_levels,list"`
	Parents          []string `parquet:"parents,list"`
	Children         []string `parquet:"children,list"`
	MinBuildingLevel string   `parquet:"min_building_level"`
	MaxBuildingLevel string   `parquet:"max_building_level"`
	SurfaceAltitude  float64  `parquet:"surface_geodetic_altitude_aster_30"`
}

func main() {
	var inputPath string
	var nodesOutputPath string
	var certificatesOutputPath string

	flag.StringVar(&inputPath, "input", "", "The path to the input .parquet file")
	flag.StringVar(&nodesOutputPath, "nodes", "", "The path to the nodes output directory")
	flag.StringVar(&certificatesOutputPath, "certs", "", "The path to certificates output directory")
	flag.Parse()

	if inputPath == "" {
		log.Fatalf("input flag must be set")
	}

	if nodesOutputPath == "" {
		log.Fatalf("'nodes' flag must be set")
	}

	if certificatesOutputPath == "" {
		log.Fatalf("'certs' flag must be set")
	}

	rows, err := parquet.ReadFile[RowType](inputPath)
	if err != nil {
		log.Fatalf("Can't open file '%s': %v", inputPath, err)
	}

	for _, c := range rows {
		fmt.Printf("%+v\n", c)
		break
	}
}
