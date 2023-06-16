package main

import (
	"flag"
	"log"

	"github.com/apache/arrow/go/v12/parquet/file"
	"github.com/apache/arrow/go/v12/parquet/schema"
)

type Coordinate struct {
	Longitude float64 `parquet:"name=lon"`
	Latitude  float64 `parquet:"name=lat"`
}
type Polygon []Coordinate
type MultiPolygon []Polygon

type Row struct {
	Domain              string         `parquet:"name=domain"`
	CertificateId       string         `parquet:"name=certificate_id"`
	ListOfMultipolygons []MultiPolygon `parquet:"name=list_of_multipolygons"`
	ListOfLevels        []string       `parquet:"name=list_of_levels"`
	Parents             []string       `parquet:"name=parents"`
	Children            []string       `parquet:"name=children"`
	MinBuildingLevel    string         `parquet:"name=min_building_level"`
	MaxBuildingLevel    string         `parquet:"name=max_building_level"`
	SurfaceAltitude     float64        `parquet:"name=surface_geodetic_altitude_aster_30"`
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

	_, err := schema.NewSchemaFromStruct(Row{})
	if err != nil {
		log.Fatalf("could not create a new schema: %v", err)
	}

	reader, err := file.OpenParquetFile(inputPath, true)
	if err != nil {
		log.Fatalf("could not open file '%s': %v", inputPath, err)
	}

	file.rec

	// schema.PrintSchema(sc.Root(), os.Stdout, 2)
}
