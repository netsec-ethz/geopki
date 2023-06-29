package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"geopki/pkg/bitstring"
	"geopki/pkg/crypto"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/schollz/progressbar/v3"
	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/reader"
)

const (
	F_GROW                        = 1
	CERTIFICATE_IMPORT_BUFFER     = 50000
	CERTIFICATE_IMPORT_BATCH_SIZE = 10000
)

type Coordinate struct {
	Lon *float64
	Lat *float64
}
type Polygon []*Coordinate
type MultiPolygon []*Polygon

func (r *MultiPolygon) GeoCertArea() crypto.GeoCertArea {
	interiorRings := make([]crypto.LinearRing, 0)
	exteriorRings := make([]crypto.LinearRing, 0)

	for _, polygon := range *r {
		ring := make([][2]float64, 0)

		for _, coordinate := range *polygon {
			ring = append(ring, [2]float64{*coordinate.Lon, *coordinate.Lat})
		}

		interiorRings = append(interiorRings, ring)
	}
	// coordinates := make([2][][][2]float64, 0)

	return crypto.GeoCertArea{
		Type:        "MultiPolygon",
		Coordinates: [2][]crypto.LinearRing{interiorRings, exteriorRings},
	}
}

const jsonSchema = `
{
  "Tag": "name=schema, repetitiontype=REQUIRED",
  "Fields": [
    {
      "Tag": "name=certificate_id, type=BYTE_ARRAY, convertedtype=UTF8, encoding=PLAIN_DICTIONARY, repetitiontype=OPTIONAL"
    },
    {
      "Tag": "name=list_of_multipolygons, type=LIST, repetitiontype=OPTIONAL",
      "Fields": [
        {
          "Tag": "name=item, type=LIST, repetitiontype=OPTIONAL",
          "Fields": [
            {
              "Tag": "name=item, type=LIST, repetitiontype=OPTIONAL",
              "Fields": [
                {
                  "Tag": "name=item, repetitiontype=OPTIONAL",
                  "Fields": [
                    {
                      "Tag": "name=lat, type=DOUBLE, repetitiontype=OPTIONAL"
                    },
                    {
                      "Tag": "name=lon, type=DOUBLE, repetitiontype=OPTIONAL"
                    }
                  ]
                }
              ]
            }
          ]
        }
      ]
    },
    {
      "Tag": "name=list_of_levels, type=LIST, repetitiontype=OPTIONAL",
      "Fields": [
        {
          "Tag": "name=item, type=BYTE_ARRAY, convertedtype=UTF8, encoding=PLAIN_DICTIONARY, repetitiontype=OPTIONAL"
        }
      ]
    },
    {
      "Tag": "name=domain, type=BYTE_ARRAY, convertedtype=UTF8, encoding=PLAIN_DICTIONARY, repetitiontype=OPTIONAL"
    },
    {
      "Tag": "name=min_building_level, type=BYTE_ARRAY, convertedtype=UTF8, encoding=PLAIN_DICTIONARY, repetitiontype=OPTIONAL"
    },
    {
      "Tag": "name=max_building_level, type=BYTE_ARRAY, convertedtype=UTF8, encoding=PLAIN_DICTIONARY, repetitiontype=OPTIONAL"
    },
    {
      "Tag": "name=parents, type=LIST, repetitiontype=OPTIONAL",
      "Fields": [
        {
          "Tag": "name=item, type=BYTE_ARRAY, convertedtype=UTF8, encoding=PLAIN_DICTIONARY, repetitiontype=OPTIONAL"
        }
      ]
    },
    {
      "Tag": "name=children, type=LIST, repetitiontype=OPTIONAL",
      "Fields": [
        {
          "Tag": "name=item, type=BYTE_ARRAY, convertedtype=UTF8, encoding=PLAIN_DICTIONARY, repetitiontype=OPTIONAL"
        }
      ]
    },
    {
      "Tag": "name=surface_altitude_aster_30, type=DOUBLE, repetitiontype=OPTIONAL"
    }
  ]
}`

type CertificateRow struct {
	Certificate_id            *string
	List_of_multipolygons     *[]*MultiPolygon
	List_of_levels            *[]*string
	Domain                    *string
	Min_building_level        *string
	Max_building_level        *string
	Parents                   *[]*string
	Children                  *[]*string
	Surface_altitude_aster_30 *float64
}

func (r *CertificateRow) NotValidAfter() string {
	return "2030-01-01 00:00:00+00"
}

func (r *CertificateRow) Certificate() (*crypto.GeoCertificate, error) {
	listOfAltitudes := make([]([2]float64), len(*r.List_of_levels))

	surfaceAltitude := 0.0
	if r.Surface_altitude_aster_30 != nil {
		surfaceAltitude = *r.Surface_altitude_aster_30
	}

	for i, level := range *r.List_of_levels {
		altitudeBounds, err := levelToAltitude(
			*r.Min_building_level,
			*r.Max_building_level,
			*level,
			surfaceAltitude,
		)
		if err != nil {
			return nil, fmt.Errorf("failed converting level to altitude: %v", err)
		}

		var bounds [2]float64
		bounds[0] = altitudeBounds.Minimum
		bounds[1] = altitudeBounds.Maximum

		listOfAltitudes[i] = bounds
	}

	areas := make([]crypto.GeoCertArea, len(*r.List_of_multipolygons))
	for i, multipolygon := range *r.List_of_multipolygons {
		areas[i] = multipolygon.GeoCertArea()
	}

	cert := crypto.GeoCertificate{
		CertificateId: *r.Certificate_id,
		Areas:         areas,
		AreasAltitude: listOfAltitudes,
		NotValidAfter: r.NotValidAfter(),
	}

	if r.Domain != nil {
		cert.Domain = *r.Domain
	}

	if len(areas) != len(listOfAltitudes) {
		fmt.Printf("%+v\n\n", r)
		return nil, fmt.Errorf("list of areas and altitudes do not have the same length. areas: %d, altitudes: %d, multipolygons: %d, levels: %d", len(areas), len(listOfAltitudes), len(*r.List_of_multipolygons), len(*r.List_of_levels))
	}

	marshaledCert, err := json.Marshal(cert)
	if err != nil {
		return nil, err
	}

	cert.MarshaledCert = marshaledCert

	return &cert, nil
}

type AltitudeBounds struct {
	Minimum float64
	Maximum float64
}

var FullAltitudeBounds = AltitudeBounds{
	Minimum: float64(bitstring.D),
	Maximum: float64(bitstring.H),
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func levelToAltitude(
	minLevel string,
	maxLevel string,
	level string,
	surfaceAltitude float64,
) (*AltitudeBounds, error) {
	if level == "@" {
		// is a node
		// no floors in this building, just use the full height
		if minLevel != "@" {
			return nil, fmt.Errorf("invariant violated, level == '@' but minLevel != '@'")
		}
		if maxLevel != "@" {
			return nil, fmt.Errorf("invariant violated, level == '@' but maxLevel != '@'")
		}

		return &FullAltitudeBounds, nil
	} else if level == "" {
		// is a building / area
		// if there are nodes within the building, min_level and max_level
		// might be set

		if minLevel == "@" && maxLevel == "@" {
			// all floors span the full building height
			// --> don't have any reference points
			return &FullAltitudeBounds, nil
		} else if minLevel == "" && maxLevel == "" {
			// some building / area without any areas / nodes within in
			return &FullAltitudeBounds, nil
		} else {
			// span the full height given by min_level and max_level
			minLevelFloat, err := strconv.ParseFloat(minLevel, 64)
			if err != nil {
				return nil, err
			}

			maxLevelFloat, err := strconv.ParseFloat(maxLevel, 64)
			if err != nil {
				return nil, err
			}

			// for now just assume one level is three meters
			// bound check in case of weirdly formatted data
			return &AltitudeBounds{
				Minimum: min(
					max(
						surfaceAltitude+minLevelFloat*3,
						float64(bitstring.D),
					),
					float64(bitstring.H),
				),
				Maximum: min(
					max(
						surfaceAltitude+(maxLevelFloat+1)*3,
						float64(bitstring.D),
					),
					float64(bitstring.H),
				),
			}, nil
		}
	} else {
		// use float(), apparently there is floor -0.5 in the dataset
		levelFloat, err := strconv.ParseFloat(level, 64)
		if err != nil {
			return nil, err
		}

		minLevelFloat, err := strconv.ParseFloat(minLevel, 64)
		if err != nil {
			return nil, err
		}

		maxLevelFloat, err := strconv.ParseFloat(maxLevel, 64)
		if err != nil {
			return nil, err
		}

		if !(minLevelFloat <= levelFloat && levelFloat <= maxLevelFloat) {
			return nil, fmt.Errorf("invariant violated: level not between min and max level")
		}

		// for now just assume one level is three meters
		// bound check in case of weirdly formatted data
		return &AltitudeBounds{
			Minimum: min(
				max(
					surfaceAltitude+levelFloat*3,
					float64(bitstring.D),
				),
				float64(bitstring.H),
			),
			Maximum: min(
				max(
					surfaceAltitude+(levelFloat+1)*3,
					float64(bitstring.D),
				),
				float64(bitstring.H),
			),
		}, nil
	}

}

func importBatch(
	address string,
	insertionKey string,
	batch []*crypto.GeoCertificate,
) error {
	certificatesJson, err := json.Marshal(batch)
	if err != nil {
		return fmt.Errorf("failed marshalling certificates: %v", err)
	}

	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return fmt.Errorf("failed creating gzip writer: %v", err)
	}

	_, err = zw.Write(certificatesJson)
	if err != nil {
		return fmt.Errorf("failed compressing certificates: %v", err)
	}

	if err := zw.Close(); err != nil {
		return fmt.Errorf("failed closing gzip writer: %v", err)
	}

	plainResponse, err := http.Post(
		fmt.Sprintf("%s/v1/insert?key=%s&is-partial=true", address, insertionKey),
		"application/json",
		bytes.NewBuffer([]byte(buf.Bytes())),
	)
	if err != nil {
		return fmt.Errorf("failed sending HTTP POST request to %s: %v", address, err)
	}

	body, err := io.ReadAll(plainResponse.Body)
	if err != nil {
		return fmt.Errorf("reading response body failed: %v", err)
	}

	if plainResponse.StatusCode != 200 {
		return fmt.Errorf("server replied with %s", string(body))
	}

	return nil
}

func certificateImporter(
	address string,
	insertionKey string,
	certificates chan *crypto.GeoCertificate,
	done chan bool,
	progressBar *progressbar.ProgressBar,
) {

	batch := make([]*crypto.GeoCertificate, 0, CERTIFICATE_IMPORT_BATCH_SIZE)

	for certificate := range certificates {
		if len(batch) >= CERTIFICATE_IMPORT_BATCH_SIZE {
			// import batch
			err := importBatch(address, insertionKey, batch)
			if err != nil {
				log.Fatalf("failed importing batch: %v\n", err)
			}

			// move progress bar
			progressBar.Add(len(batch))
			// keep allocated memory but empty slice
			batch = batch[:0]
		} else {
			batch = append(batch, certificate)
		}
	}

	if len(batch) > 0 {
		// import batch

		err := importBatch(address, insertionKey, batch)
		if err != nil {
			log.Fatalf("failed importing last batch: %v\n", err)
		}

		// move progress bar
		progressBar.Add(len(batch))
	}

	progressBar.Exit()

	done <- true
}

func main() {
	var inputPath string
	var address string
	var insertionKey string

	flag.StringVar(&inputPath, "input", "", "The path to the input .parquet file")
	flag.StringVar(&address, "address", "http://localhost:1234", "The address of the server where the certificates should be imported")
	flag.StringVar(&insertionKey, "insertion-key", "", "The insetion key")
	flag.Parse()

	if inputPath == "" {
		log.Fatalf("input flag must be set")
	}

	if insertionKey == "" {
		log.Fatalf("insertion key must be set")
	}

	fileReader, err := local.NewLocalFileReader(inputPath)
	if err != nil {
		log.Println("Can't open file", err)
		return
	}
	pr, err := reader.NewParquetReader(fileReader, jsonSchema, 4)
	if err != nil {
		log.Println("Can't create parquet reader", err)
		return
	}

	num := int(pr.GetNumRows())
	progressBar := progressbar.Default(int64(num), "locate certificates")

	// setup channels for concurrently writing certificates to disk
	certificates := make(chan *crypto.GeoCertificate, CERTIFICATE_IMPORT_BUFFER)
	certificatesFinishedImporting := make(chan bool)
	// start writer routine
	go certificateImporter(
		address,
		insertionKey,
		certificates,
		certificatesFinishedImporting,
		progressBar,
	)

	for i := 0; i < num; i++ {
		rows := make([]CertificateRow, 1)
		if err = pr.Read(&rows); err != nil {
			log.Println("Read error", err)
		}
		r := rows[0]

		if r.Domain == nil {
			progressBar.Add(1)
			continue
		}

		certificate, err := r.Certificate()
		if err != nil {
			log.Fatalf("failed converting to a certificate: %v", err)
		}

		certificates <- certificate
	}
	// tell certificate importer that it has all certificates
	close(certificates)

	// wait for the importer to finish
	<-certificatesFinishedImporting
	fmt.Printf("Imported all certificates, recompute hashes now.\n")

	start := time.Now()
	fmt.Printf("This can take quite some time..\n")

	plainResponse, err := http.Get(
		fmt.Sprintf("%s/v1/recompute-hashes", address),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed sending HTTP GET request to %s: %v", address, err)
		os.Exit(1)
	}

	body, err := io.ReadAll(plainResponse.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading response body failed: %v\n", err)
		os.Exit(1)
	}

	if plainResponse.StatusCode != 200 {
		fmt.Fprintf(os.Stderr, "server replied with %s", string(body))
		os.Exit(1)
	}

	fmt.Printf("Done, re-computed all hashes in %f minutes.\n", time.Since(start).Minutes())
}
