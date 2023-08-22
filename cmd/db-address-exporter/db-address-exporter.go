package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"geopki/pkg/bitstring"
	"geopki/pkg/crypto"
	"geopki/pkg/geometry"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/schollz/progressbar/v3"
	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/reader"
)

const (
	// the relative grid size used to assign certificates to SMT nodes
	F_GROW = 0.1

	// the number of certificates, nodes, samples, certificate sizes and altitude
	// values that fit in the buffer. after exceeding this number, the main
	// process blocks until the respective writer process makes some progress.
	CERTIFICATE_WRITE_BUFFER            = 1000
	NODE_WRITE_BUFFER                   = 1000
	SAMPLE_WRITE_BUFFER                 = 1000
	CERT_SIZE_DISTRIBUTION_WRITE_BUFFER = 1000
	ALTITUDE_DISTRIBUTION_WRITE_BUFFER  = 1000

	// the maximum file size for .sql files, after exceeding this limit a new file is created
	MAX_FILE_SIZE = 300 * 1000 * 1000 // 300 MB
	// the maximum certificate file size in bytes. any certificate larger than this value is discarded
	MAX_CERTIFICATE_SIZE = 3328 // 3.25KiB / 99% is below this

	// string constants used in the generated .sql files
	INSERT_INTO_NODES_STR = "INSERT INTO nodes(bit_string_51,bit_string_15,xy_left_child_hash,xy_right_child_hash,z_left_child_hash,z_right_child_hash,certificate_hashes) VALUES\n"
	INSERT_INTO_CERTS_STR = "INSERT INTO certificates(certificate_hash,certificate,not_valid_after) VALUES\n"

	// the schema used to parse the .parquet dataset
	jsonSchema = `
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
)

// a coordinate tuple consisting of a [Lon]gitude and a [Lat]itude
type Coordinate struct {
	Lon *float64
	Lat *float64
}

// a polygon represented by a list of coordinates (ccw order), without any holes
type Polygon []*Coordinate

// a multi-polygon aka, a list of polygons
type MultiPolygon []*Polygon

// transforms a multi-polygon into a geocert area. in the current implementation
// this follows the geojson standard
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

// the type of a row from the .parquet file, corresponding to `jsonSchema`
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

// the certificate's expiration date
func (r *CertificateRow) NotValidAfter() string {
	// a constant for now
	return "2030-01-01 00:00:00+00"
}

// transforms the certificate row into a geo certificate
func (r *CertificateRow) Certificate() (*crypto.GeoCertificate, error) {
	// default altitude value is 0
	surfaceAltitude := 0.0
	if r.Surface_altitude_aster_30 != nil {
		// if the row stores a surface altitude value, use this
		surfaceAltitude = *r.Surface_altitude_aster_30
	}

	// a list of minimum and maximum altitude values, one for each floor level
	listOfAltitudes := make([]([2]float64), len(*r.List_of_levels))

	// for each level determine the minimum and maximum altitude values
	// fill `listOfAltitudes` with these values
	for i, level := range *r.List_of_levels {
		// use heuristic to approximate the altitude bounds
		altitudeBounds, err := levelToAltitude(
			*r.Min_building_level,
			*r.Max_building_level,
			*level,
			surfaceAltitude,
		)
		if err != nil {
			return nil, fmt.Errorf("failed converting level to altitude: %v", err)
		}

		// transform altitude values to the proper datatype
		var bounds [2]float64
		bounds[0] = altitudeBounds.Minimum
		bounds[1] = altitudeBounds.Maximum

		listOfAltitudes[i] = bounds
	}

	// turn each multi-polygon into a geo cert area
	areas := make([]crypto.GeoCertArea, len(*r.List_of_multipolygons))
	for i, multipolygon := range *r.List_of_multipolygons {
		areas[i] = multipolygon.GeoCertArea()
	}

	// finally, create a geo cert instance using the values from above
	cert := crypto.GeoCertificate{
		CertificateId: *r.Certificate_id,
		Areas:         areas,
		AreasAltitude: listOfAltitudes,
		NotValidAfter: r.NotValidAfter(),
	}

	// optionally set the geo cert's domain
	if r.Domain != nil {
		cert.Domain = *r.Domain
	}

	// perform a sanity check, the number of areas an altitude bounds must match, i.e.
	// one altitude bound per multi-polygon / area
	if len(areas) != len(listOfAltitudes) {
		fmt.Printf("%+v\n\n", r)
		return nil, fmt.Errorf("list of areas and altitudes do not have the same length. areas: %d, altitudes: %d, multipolygons: %d, levels: %d", len(areas), len(listOfAltitudes), len(*r.List_of_multipolygons), len(*r.List_of_levels))
	}

	// set the certificate's `MarshaledCert` field. in a proper implementation
	// this field should be computed but the problem is that multiple json representations
	// are equivalent but the signatures over equivalent json representations are not
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

// AltitudeBounds instance covering the whole range
var FullAltitudeBounds = AltitudeBounds{
	Minimum: float64(bitstring.D),
	Maximum: float64(bitstring.H),
}

// returns the minimum of two float64 values
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// returns the maximum of two float64 values
func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// heuristic for approximating the altitude bounds of a floor
func levelToAltitude(
	// the building's minimum floor level
	minLevel string,
	// the building's maximum floor level
	maxLevel string,
	// the floor's level
	level string,
	// the surface altitude value
	surfaceAltitude float64,
) (*AltitudeBounds, error) {
	if level == "@" {
		// is a node and there are no floors in this building, just use the full height
		if minLevel != "@" {
			return nil, fmt.Errorf("invariant violated, level == '@' but minLevel != '@'")
		}
		if maxLevel != "@" {
			return nil, fmt.Errorf("invariant violated, level == '@' but maxLevel != '@'")
		}

		return &FullAltitudeBounds, nil
	} else if level == "" {
		// this is a building / area
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

// encodes a hash in a way suitable for postgres
func encodeHashForDatabase(hash crypto.SHA256Hash) string {
	if hash == nil {
		return "NULL"
	} else {
		return fmt.Sprintf("E'\\\\x%s'", hex.EncodeToString(hash))
	}
}

// go-routine for writing .sql files containing 'INSERT INTO certificates' statements
func certificateWriter(
	// the path to the output directory
	certificatesOutputPath string,
	// a channel for passing the geo certificates that should be written to disk
	certificates chan *crypto.GeoCertificate,
	// a channel used to indicate that the writer terminated
	done chan bool,
) {
	// a unique index for each .sql file
	fileIndex := 0

	// the path the next file is written to
	fileName := fmt.Sprintf("%s/part-%d.sql", certificatesOutputPath, fileIndex)

	// the size of the current file
	size := 0

	// whether the next line will be the first line written to this file, could also just check size != 0
	isFirstLine := true

	// create current file to write to
	file, err := os.Create(fileName)
	if err != nil {
		log.Fatalf("failed opening file %s: %v", fileName, err)
	}

	// read from the channel until it is closed
	for certificate := range certificates {
		// temporary variable for the number of written bytes
		var n int
		var err error

		if isFirstLine {
			// if it is the first line, write an "INSERT INTO" statement
			n, err = file.Write([]byte(INSERT_INTO_CERTS_STR))
			isFirstLine = false
		} else {
			// otherwise just add a comma and a line break
			n, err = file.Write([]byte(",\n"))
		}
		if err != nil {
			log.Fatalf("failed writing to file %s: %v", fileName, err)
		}

		// keep track of the current file's size
		size += n

		// now write the certificate row values
		n, err = file.Write(
			[]byte(
				fmt.Sprintf(
					"(%s, %s, '%s')",
					encodeHashForDatabase(certificate.Hash()),
					encodeHashForDatabase(certificate.MarshaledCert),
					certificate.NotValidAfter,
				),
			),
		)
		if err != nil {
			log.Fatalf("failed writing to file %s: %v", fileName, err)
		}

		// keep track of the current file's size
		size += n

		// if with this row we exceed the maximum file size
		if size > MAX_FILE_SIZE {
			// make sure duplicates are ignored
			_, err = file.Write([]byte("\nON CONFLICT (certificate_hash) DO NOTHING"))
			if err != nil {
				log.Fatalf("failed writing to file %s: %v", fileName, err)
			}

			// and close the file
			err = file.Close()
			if err != nil {
				log.Fatalf("failed closing file %s: %v", fileName, err)
			}

			// finally, reset variables for the next iteration
			fileIndex += 1
			fileName := fmt.Sprintf("%s/part-%d.sql", certificatesOutputPath, fileIndex)
			file, err = os.Create(fileName)
			if err != nil {
				log.Fatalf("failed opening file %s: %v", fileName, err)
			}

			size = 0
			isFirstLine = true
		}
	}

	if !isFirstLine {
		// finish the file correctly analogous to when the maximum file size is reached
		_, err = file.Write([]byte("\nON CONFLICT (certificate_hash) DO NOTHING"))
		if err != nil {
			log.Fatalf("failed writing to file %s: %v", fileName, err)
		}
	}

	// close the file
	err = file.Close()
	if err != nil {
		log.Fatalf("failed closing file %s: %v", fileName, err)
	}

	if isFirstLine {
		// and if the file is empty, delete it
		err := os.Remove(fileName)
		if err != nil {
			log.Fatalf("failed removing empty file %s: %v", fileName, err)
		}
	}

	// indicate the go-routine has terminated
	done <- true
}

// go-routine for writing .sql files containing 'INSERT INTO nodes' statements
func nodeWriter(
	// the path to the output directory
	nodesOutputPath string,
	// a channel for passing the SMT nodes that should be written to disk
	nodes chan *crypto.Node,
	// a channel used to indicate that the writer terminated
	done chan bool,
) {
	// a unique index for each .sql file
	fileIndex := 0

	// the path the next file is written to
	fileName := fmt.Sprintf("%s/part-%d.sql", nodesOutputPath, fileIndex)

	// the size of the current file
	size := 0

	// whether the next line will be the first line written to this file, could also just check size != 0
	isFirstLine := true

	// create current file to write to
	file, err := os.Create(fileName)
	if err != nil {
		log.Fatalf("failed opening file %s: %v", fileName, err)
	}

	// read from the channel until it is closed
	for node := range nodes {
		// temporary variable for the number of written bytes
		var n int
		var err error

		if isFirstLine {
			// if it is the first line, write an "INSERT INTO" statement
			n, err = file.Write([]byte(INSERT_INTO_NODES_STR))
			isFirstLine = false
		} else {
			// otherwise just add a comma and a line break
			n, err = file.Write([]byte(",\n"))
		}
		if err != nil {
			log.Fatalf("failed writing to file %s: %v", fileName, err)
		}

		// keep track of the current file's size
		size += n

		// transform the array of certifcate hashes into a postgres-compatible format
		certificateHashes := make([]string, len(node.CertificateHashes))
		for i, certificateHash := range node.SortedCertificateHashes() {
			certificateHashes[i] = encodeHashForDatabase(certificateHash)
		}
		certificateHashArray := "ARRAY[" + strings.Join(certificateHashes, ",") + "]::bytea[]"

		// now write the SMT node row values
		n, err = file.Write(
			[]byte(
				fmt.Sprintf(
					"(b'%s', b'%s', %s, %s, %s, %s, %s)",
					node.RawXYBitString.BitString().String(),
					node.RawZBitString.BitString().String(),
					encodeHashForDatabase(node.XYLeftChildHash(false)),
					encodeHashForDatabase(node.XYRightChildHash(false)),
					encodeHashForDatabase(node.ZLeftChildHash(false)),
					encodeHashForDatabase(node.ZRightChildHash(false)),
					certificateHashArray,
				),
			),
		)
		if err != nil {
			log.Fatalf("failed writing to file %s: %v", fileName, err)
		}

		// keep track of the current file's size
		size += n

		// if with this row we exceed the maximum file size
		if size > MAX_FILE_SIZE {
			// close the file
			err = file.Close()
			if err != nil {
				log.Fatalf("failed closing file %s: %v", fileName, err)
			}

			// reset variables for the next iteration
			fileIndex += 1
			fileName := fmt.Sprintf("%s/part-%d.sql", nodesOutputPath, fileIndex)
			file, err = os.Create(fileName)
			if err != nil {
				log.Fatalf("failed opening file %s: %v", fileName, err)
			}

			size = 0
			isFirstLine = true
		}
	}

	// after writing the last SMT node, close the current file
	err = file.Close()
	if err != nil {
		log.Fatalf("failed closing file %s: %v", fileName, err)
	}

	if isFirstLine {
		// and if the current file is empty, delete it
		err := os.Remove(fileName)
		if err != nil {
			log.Fatalf("failed removing empty file %s: %v", fileName, err)
		}
	}

	// indicate the go-routine has terminated
	done <- true
}

// go-routine for writing a .csv file containing one coordinate of each certificate
func sampleWriter(
	// the .csv output filename
	fileName string,
	// a channel for passing the coordinates that should be written to disk
	samples chan *Coordinate,
	// a channel used to indicate that the writer terminated
	done chan bool,
) {
	// create the output file
	file, err := os.Create(fileName)
	if err != nil {
		log.Fatalf("failed opening file %s: %v", fileName, err)
	}

	// write the csv header
	_, err = file.Write(
		[]byte("longitude,latitude\n"),
	)
	if err != nil {
		log.Fatalf("failed writing header to %s: %v", fileName, err)
	}

	// read from the channel until it is closed
	for sample := range samples {

		// write csv coordinate row
		_, err := file.Write(
			[]byte(
				fmt.Sprintf(
					"%f,%f\n",
					*sample.Lon,
					*sample.Lat,
				),
			),
		)
		if err != nil {
			log.Fatalf("failed writing to file %s: %v", fileName, err)
		}
	}

	// close the file
	err = file.Close()
	if err != nil {
		log.Fatalf("failed closing file %s: %v", fileName, err)
	}

	// indicate the go-routine has terminated
	done <- true
}

// go-routine for writing a .csv file containing each certificate's size
func certSizeWriter(
	// the .csv output filename
	fileName string,
	// a channel for passing the certificate sizes that should be written to disk
	certificateSizes chan int,
	// a channel used to indicate that the writer terminated
	done chan bool,
) {
	// create the output file
	file, err := os.Create(fileName)
	if err != nil {
		log.Fatalf("failed opening file %s: %v", fileName, err)
	}

	// write the csv header
	_, err = file.Write(
		[]byte("length\n"),
	)
	if err != nil {
		log.Fatalf("failed writing header to %s: %v", fileName, err)
	}

	// read from the channel until it is closed
	for certificateSize := range certificateSizes {
		// write the certificate size
		_, err := file.Write(
			[]byte(
				fmt.Sprintf(
					"%d\n",
					certificateSize,
				),
			),
		)
		if err != nil {
			log.Fatalf("failed writing to file %s: %v", fileName, err)
		}
	}

	// close the file
	err = file.Close()
	if err != nil {
		log.Fatalf("failed closing file %s: %v", fileName, err)
	}

	// indicate the go-routine has terminated
	done <- true
}

// go-routine for writing a .csv file containing each certificate's surface altitude
func surfaceAltitudeWriter(
	// the .csv output filename
	fileName string,
	// a channel for passing the altitude values that should be written to disk
	surfaceAltitudes chan *float64,
	// a channel used to indicate that the writer terminated
	done chan bool,
) {
	// create the output file
	file, err := os.Create(fileName)
	if err != nil {
		log.Fatalf("failed opening file %s: %v", fileName, err)
	}

	// write the csv header
	_, err = file.Write(
		[]byte("altitude\n"),
	)
	if err != nil {
		log.Fatalf("failed writing header to %s: %v", fileName, err)
	}

	// read from the channel until it is closed
	for surfaceAltitude := range surfaceAltitudes {
		// write the altitude value
		_, err := file.Write(
			[]byte(
				fmt.Sprintf(
					"%f\n",
					*surfaceAltitude,
				),
			),
		)
		if err != nil {
			log.Fatalf("failed writing to file %s: %v", fileName, err)
		}
	}

	// close the file
	err = file.Close()
	if err != nil {
		log.Fatalf("failed closing file %s: %v", fileName, err)
	}

	// indicate the go-routine has terminated
	done <- true
}

func main() {
	// the CLI arguments described by the help messages below
	var inputPath string
	var nodesOutputPath string
	var certificatesOutputPath string
	var sampleOutputPath string
	var certSizeOutputPath string
	var surfaceAltitudeOutputPath string

	flag.StringVar(&inputPath, "input", "", "The path to the input .parquet file")
	flag.StringVar(&nodesOutputPath, "nodes", "", "The path to the nodes output directory")
	flag.StringVar(&certificatesOutputPath, "certs", "", "The path to certificates output directory")
	flag.StringVar(&sampleOutputPath, "sampling-map", "", "The path to the sampling-map file")
	flag.StringVar(&certSizeOutputPath, "cert-size-dist", "", "The path to the certificate size distribution file")
	flag.StringVar(&surfaceAltitudeOutputPath, "altitude-dist", "", "The path to the altitude distribution file")
	flag.Parse()

	// make sure the required CLI arguments are set
	if inputPath == "" {
		log.Fatalf("'input' flag must be set")
	}

	if nodesOutputPath == "" {
		log.Fatalf("'nodes' flag must be set")
	}

	if certificatesOutputPath == "" {
		log.Fatalf("'certs' flag must be set")
	}

	if sampleOutputPath == "" {
		log.Fatalf("'sample-map' flag must be set")
	}

	if certSizeOutputPath == "" {
		log.Fatalf("'cert-size-dist' flag must be set")
	}

	if surfaceAltitudeOutputPath == "" {
		log.Fatalf("'altitude-dist' flag must be set")
	}

	// create file reader for the .parquet dataset
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

	// initialize a progress bar
	num := int(pr.GetNumRows())
	progressBar := progressbar.Default(int64(num), "locate certificates")

	// setup channels for the different go-routines writing data to disk

	// setup channels for concurrently writing certificates to disk
	certificates := make(chan *crypto.GeoCertificate, CERTIFICATE_WRITE_BUFFER)
	certificatesFinishedWriting := make(chan bool)
	// start writer routine
	go certificateWriter(
		certificatesOutputPath,
		certificates,
		certificatesFinishedWriting,
	)

	// setup channels for concurrently writing samples to disk
	samples := make(chan *Coordinate, SAMPLE_WRITE_BUFFER)
	samplesFinishedWriting := make(chan bool)
	// start writer routine
	go sampleWriter(
		sampleOutputPath,
		samples,
		samplesFinishedWriting,
	)

	// setup channels for concurrently writing the certificate size distribution to disk
	certificateSizes := make(chan int, CERT_SIZE_DISTRIBUTION_WRITE_BUFFER)
	certSizesFinishedWriting := make(chan bool)
	// start writer routine
	go certSizeWriter(
		certSizeOutputPath,
		certificateSizes,
		certSizesFinishedWriting,
	)

	// setup channels for concurrently writing the surface altitude distribution to disk
	altitudes := make(chan *float64, ALTITUDE_DISTRIBUTION_WRITE_BUFFER)
	altitudeFinishedWriting := make(chan bool)
	// start writer routine
	go surfaceAltitudeWriter(
		surfaceAltitudeOutputPath,
		altitudes,
		altitudeFinishedWriting,
	)

	// create one gigantic map, mapping from a bit string pair to an SMT node
	bitstringPairToNode := make(map[bitstring.RawBitStringPair]*crypto.Node)

	// then iterate over each row in the .parquet file
	for i := 0; i < num; i++ {
		// read a row
		rows := make([]CertificateRow, 1)
		if err = pr.Read(&rows); err != nil {
			log.Println("Read error", err)
		}
		r := rows[0]

		// skip the row if it does not contain a domain (e.g. larger areas, buildings, floors, etc.)
		if r.Domain == nil {
			progressBar.Add(1)
			continue
		}

		// compute the geo cert based on the row data
		certificate, err := r.Certificate()
		if err != nil {
			log.Fatalf("failed converting to a certificate: %v", err)
		}

		// skip the row if the geo certificate is greater than the maximum certificate size
		certificateSizes <- len(certificate.MarshaledCert)
		if len(certificate.MarshaledCert) > MAX_CERTIFICATE_SIZE {
			progressBar.Add(1)
			continue
		}

		// point of no return: all certificates passing the checks up to here
		// are exported

		// get some coordinate of the mulitpolygon and write it to disk
		coordinate := (*(*(*r.List_of_multipolygons)[0])[0])[0]
		samples <- coordinate

		// if the row has a surface altitude value, write it to disk
		if r.Surface_altitude_aster_30 != nil {
			altitudes <- r.Surface_altitude_aster_30
		}

		// compute the set of SMT nodes for the certificate
		bitstringPairs, err := geometry.CertificateToBitStrings(certificate, F_GROW)
		if err != nil {
			// fmt.Printf("==%s== \n", *r.Certificate_id)
			log.Fatalf("failed converting to bit string pairs: %v", err)
		}

		// assign this geo certificate for to each SMT node represented by these bit string pairs
		for _, bitstringPair := range bitstringPairs {
			// add certificate to the SMT
			node, ok := bitstringPairToNode[*bitstringPair]

			if ok {
				// if this SMT node already exists, append the certificate hashes
				node.CertificateHashes = append(node.CertificateHashes, certificate.Hash())
			} else {
				// otherwise create a new SMT node
				bitstringPairToNode[*bitstringPair] = crypto.NewNode(
					bitstringPair.XYBitString,
					bitstringPair.XYBitStringLen,
					bitstringPair.ZBitString,
					bitstringPair.ZBitStringLen,
					nil, nil, nil, nil,
					[]crypto.SHA256Hash{certificate.Hash()},
				)
			}

			// iterate over all prefixes of that bit string (ancestors) and add them to bitstringPairToNode

			// first iterate over prefixes of the altitude bit string, including the empty string ''
			for i := uint8(1); i <= bitstringPair.RawZBitString.ZBitStringLen; i++ {
				ancestor := bitstringPair.RawZBitString.Ancestor(i)
				b := bitstring.RawBitStringPair{
					RawXYBitString: bitstringPair.RawXYBitString,
					RawZBitString:  ancestor,
				}

				_, ok := bitstringPairToNode[b]
				if !ok {
					// if that ancestor doesn't exist yet, create a new, empty SMT node
					bitstringPairToNode[b] = crypto.NewNode(
						b.XYBitString,
						b.XYBitStringLen,
						b.ZBitString,
						b.ZBitStringLen,
						nil, nil, nil, nil,
						nil,
					)
				}
			}

			// next iterate over prefixes of the 2D bit string including the empty string ''
			for i := uint8(1); i <= bitstringPair.RawXYBitString.XYBitStringLen; i++ {
				ancestor := bitstringPair.RawXYBitString.Ancestor(i)
				b := bitstring.RawBitStringPair{
					RawXYBitString: ancestor,
					RawZBitString: bitstring.RawZBitString{
						ZBitString:    0,
						ZBitStringLen: 0,
					},
				}

				_, ok := bitstringPairToNode[b]
				if !ok {
					bitstringPairToNode[b] = crypto.NewNode(
						b.XYBitString,
						b.XYBitStringLen,
						b.ZBitString,
						b.ZBitStringLen,
						nil, nil, nil, nil,
						nil,
					)
				}
			}
		}

		// write certificate to disk
		certificates <- certificate
		progressBar.Add(1)
	}

	// tell go-routines that they have all data
	close(certificates)
	close(samples)
	close(certificateSizes)
	close(altitudes)

	// finish progress bar
	progressBar.Exit()

	// compute child hashes

	// create slice containing the map's keys
	bitstrings := make([]bitstring.RawBitStringPair, 0, len(bitstringPairToNode))
	for bitstringPair := range bitstringPairToNode {
		bitstrings = append(bitstrings, bitstringPair)
	}

	// sort bit strings in ascending order
	sort.Slice(bitstrings, func(i, j int) bool {
		// must return true if i is smaller than j (smaller = has longer bit strings)
		return ((bitstrings[i].XYBitStringLen > bitstrings[j].XYBitStringLen) || (bitstrings[i].XYBitStringLen == bitstrings[j].XYBitStringLen && bitstrings[i].ZBitStringLen > bitstrings[j].ZBitStringLen))
	})

	// setup channels for concurrently writing nodes to disk
	nodes := make(chan *crypto.Node, NODE_WRITE_BUFFER)
	nodesFinishedWriting := make(chan bool)

	// start writer routine
	go nodeWriter(
		nodesOutputPath,
		nodes,
		nodesFinishedWriting,
	)

	// initialize new porgress bar
	progressBar = progressbar.Default(int64(len(bitstrings)), "compute hashes")

	// iterate over all SMT nodes and write them to disk
	for _, bitstring := range bitstrings {
		node := bitstringPairToNode[bitstring]

		xyLeftChild, err := bitstring.XYLeftChildPair()
		if err == nil {
			xyLeftChildNode, ok := bitstringPairToNode[xyLeftChild]

			if ok {
				node.SetXYLeftChildHash(xyLeftChildNode.Hash())
			}
		}
		// err = no xy left child

		xyRightChild, err := bitstring.XYRightChildPair()
		if err == nil {
			xyRightChildNode, ok := bitstringPairToNode[xyRightChild]

			if ok {
				node.SetXYRightChildHash(xyRightChildNode.Hash())
			}
		}
		// err = no xy right child

		zLeftChildNode, ok := bitstringPairToNode[bitstring.ZLeftChildPair()]
		if ok {
			node.SetZLeftChildHash(zLeftChildNode.Hash())
		}

		zRightChildNode, ok := bitstringPairToNode[bitstring.ZRightChildPair()]
		if ok {
			node.SetZRightChildHash(zRightChildNode.Hash())
		}

		// write SMT node to disk
		nodes <- node
		progressBar.Add(1)
	}
	// tell the go-routine no further data will arrive
	close(nodes)

	// finish progress bar
	progressBar.Exit()

	// wait for all go-routines to finish writing data to disk
	fmt.Printf("Waiting until all samples are written to disk..\n")
	<-samplesFinishedWriting
	fmt.Printf("Done!\n")

	fmt.Printf("Waiting until all certificate size values are written to disk..\n")
	<-certSizesFinishedWriting
	fmt.Printf("Done!\n")

	fmt.Printf("Waiting until all altitude values are written to disk..\n")
	<-altitudeFinishedWriting
	fmt.Printf("Done!\n")

	fmt.Printf("Waiting until all certificates are written to disk..\n")
	<-certificatesFinishedWriting
	fmt.Printf("Done!\n")

	fmt.Printf("Waiting until all nodes are written to disk..\n")
	<-nodesFinishedWriting
	fmt.Printf("Done!\n")
}
