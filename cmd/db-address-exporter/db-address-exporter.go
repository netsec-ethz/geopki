package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"geopki/pkg/bitstring"
	"geopki/pkg/crypto"
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
	F_GROW                = 1
	MAX_FILE_SIZE         = 300 * 1000 * 1000 // 300 MB
	INSERT_INTO_NODES_STR = "INSERT INTO nodes(bit_string_51,bit_string_51_int,bit_string_15,altitude_min,altitude_max,xy_left_child_hash,xy_right_child_hash,z_left_child_hash,z_right_child_hash,certificate_hashes) VALUES\n"
	INSERT_INTO_CERTS_STR = "INSERT INTO certificates(certificate_hash,certificate,not_valid_after) VALUES\n"
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
      "Tag": "name=surface_geodetic_altitude_aster_30, type=DOUBLE, repetitiontype=OPTIONAL"
    }
  ]
}`

type CertificateRow struct {
	Certificate_id                     *string
	List_of_multipolygons              *[]*MultiPolygon
	List_of_levels                     *[]*string
	Domain                             *string
	Min_building_level                 *string
	Max_building_level                 *string
	Parents                            *[]*string
	Children                           *[]*string
	Surface_geodetic_altitude_aster_30 *float64
}

func (r *CertificateRow) NotValidAfter() string {
	return "2030-01-01 00:00:00+00"
}

func (r *CertificateRow) Certificate() (*crypto.GeoCertificate, error) {
	listOfAltitudes := make([]([2]float64), len(*r.List_of_levels))

	surfaceAltitude := 0.0
	if r.Surface_geodetic_altitude_aster_30 != nil {
		surfaceAltitude = *r.Surface_geodetic_altitude_aster_30
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

func encodeHashForDatabase(hash crypto.SHA256Hash) string {
	if hash == nil {
		return "NULL"
	} else {
		return fmt.Sprintf("E'\\\\x%s'", hex.EncodeToString(hash))
	}
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

	certificates := make([]*crypto.GeoCertificate, 0, num)
	bitstringPairToNode := make(map[bitstring.RawBitStringPair]*crypto.Node)

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

		bitstringPairs, err := certificate.BitStrings(F_GROW)
		if err != nil {
			if strings.Contains(err.Error(), "invalid loop") || strings.Contains(err.Error(), "duplicate vertices") {
				// if the geometry is invalid, ignore the certificate
				progressBar.Add(1)
				continue
			}
			// fmt.Printf("==%s== \n", *r.Certificate_id)
			log.Fatalf("failed converting to bit string pairs: %v", err)
		}

		for _, bitstringPair := range bitstringPairs {
			// add certificate to the tree
			node, ok := bitstringPairToNode[*bitstringPair]
			if ok {
				node.CertificateHashes = append(node.CertificateHashes, certificate.Hash())
			} else {
				bitstringPairToNode[*bitstringPair] = crypto.NewNode(
					bitstringPair.XYBitString,
					bitstringPair.XYBitStringLen,
					bitstringPair.ZBitString,
					bitstringPair.ZBitStringLen,
					nil, nil, nil, nil,
					[]crypto.SHA256Hash{certificate.Hash()},
				)
			}

			// iterate over all prefixes of that bit string and add them to bitstringPairToNode
			// first iterate over prefixes of the altitude bit string, including the empty string ''
			for i := uint8(1); i <= bitstringPair.RawZBitString.ZBitStringLen; i++ {
				ancestor := bitstringPair.RawZBitString.Ancestor(i)
				b := bitstring.RawBitStringPair{
					RawXYBitString: bitstringPair.RawXYBitString,
					RawZBitString:  ancestor,
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

			// next iterate over prefixes of the 2D bit string
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

		certificates = append(certificates, certificate)
		progressBar.Add(1)
	}
	progressBar.Exit()

	// compute child hashes
	bitstrings := make([]bitstring.RawBitStringPair, 0, len(bitstringPairToNode))
	for bitstringPair := range bitstringPairToNode {
		bitstrings = append(bitstrings, bitstringPair)
	}

	// sort bit strings in ascending order
	sort.Slice(bitstrings, func(i, j int) bool {
		// must return true if i is smaller than j (smaller = has longer bit strings)
		return ((bitstrings[i].XYBitStringLen > bitstrings[j].XYBitStringLen) || (bitstrings[i].XYBitStringLen == bitstrings[j].XYBitStringLen && bitstrings[i].ZBitStringLen > bitstrings[j].ZBitStringLen))
	})

	progressBar = progressbar.Default(int64(len(bitstrings)), "compute hashes")

	for _, bitstring := range bitstrings {
		node := bitstringPairToNode[bitstring]

		xyLeftChild, err := bitstring.XYLeftChildPair()
		if err == nil {
			xyLeftChildNode, ok := bitstringPairToNode[xyLeftChild]

			if ok {
				node.SetXYLeftChildHash(xyLeftChildNode.Hash())
			}
		}
		// ignore else case, might not have a xy left child

		xyRightChild, err := bitstring.XYRightChildPair()
		if err == nil {
			xyRightChildNode, ok := bitstringPairToNode[xyRightChild]

			if ok {
				node.SetXYRightChildHash(xyRightChildNode.Hash())
			}
		}
		// ignore else case, might not have a xy right child

		zLeftChildNode, ok := bitstringPairToNode[bitstring.ZLeftChildPair()]
		if ok {
			node.SetZLeftChildHash(zLeftChildNode.Hash())
		}

		zRightChildNode, ok := bitstringPairToNode[bitstring.ZRightChildPair()]
		if ok {
			node.SetZLeftChildHash(zRightChildNode.Hash())
		}

		progressBar.Add(1)
	}
	progressBar.Exit()

	// write output
	fileIndex := 0
	fileName := fmt.Sprintf("%s/part-%d.sql", nodesOutputPath, fileIndex)
	size := 0
	isFirstLine := true

	file, err := os.Create(fileName)
	if err != nil {
		log.Fatalf("failed opening file %s: %v", fileName, err)
	}

	progressBar = progressbar.Default(int64(len(bitstrings)), "write nodes")
	for _, bitstring := range bitstrings {
		var n int
		var err error

		node := bitstringPairToNode[bitstring]

		if isFirstLine {
			// write insert statement
			n, err = file.Write([]byte(INSERT_INTO_NODES_STR))
			isFirstLine = false
		} else {
			// insert line break
			n, err = file.Write([]byte("\n"))
		}
		if err != nil {
			log.Fatalf("failed writing to file %s: %v", fileName, err)
		}
		size += n

		certificateHashes := make([]string, len(node.CertificateHashes))
		for i, certificateHash := range node.SortedCertificateHashes() {
			certificateHashes[i] = encodeHashForDatabase(certificateHash)
		}
		certificateHashArray := "ARRAY[" + strings.Join(certificateHashes, ",") + "]::bytea[]"

		// write node row
		n, err = file.Write(
			[]byte(
				fmt.Sprintf(
					"(b'%s', %d, b'%s', %d, %d, %s, %s, %s, %s, %s)",
					node.RawXYBitString.BitString().String(),
					node.XYBitString>>(64-51),
					node.RawZBitString.BitString().String(),
					node.RawZBitString.BitString().ZMin,
					node.RawZBitString.BitString().ZMax(),
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
		size += n

		if size > MAX_FILE_SIZE {
			err = file.Close()
			if err != nil {
				log.Fatalf("failed closing file %s: %v", fileName, err)
			}

			fileIndex += 1
			fileName := fmt.Sprintf("%s/part-%d.sql", nodesOutputPath, fileIndex)
			file, err = os.Create(fileName)
			if err != nil {
				log.Fatalf("failed opening file %s: %v", fileName, err)
			}

			size = 0
			isFirstLine = true
		}

		progressBar.Add(1)
	}
	progressBar.Exit()

	err = file.Close()
	if err != nil {
		log.Fatalf("failed closing file %s: %v", fileName, err)
	}

	fileIndex = 0
	fileName = fmt.Sprintf("%s/part-%d.sql", certificatesOutputPath, fileIndex)
	size = 0
	isFirstLine = true
	file, err = os.Create(fileName)
	if err != nil {
		log.Fatalf("failed opening file %s: %v", fileName, err)
	}

	progressBar = progressbar.Default(int64(len(certificates)), "write certificates")
	for _, certificate := range certificates {
		var n int
		var err error

		if isFirstLine {
			// write insert statement
			n, err = file.Write([]byte(INSERT_INTO_CERTS_STR))
			isFirstLine = false
		} else {
			// insert line break
			n, err = file.Write([]byte("\n"))
		}
		if err != nil {
			log.Fatalf("failed writing to file %s: %v", fileName, err)
		}
		size += n

		// write certificate row
		n, err = file.Write(
			[]byte(
				fmt.Sprintf(
					"(%s, %s, '%s')",
					encodeHashForDatabase(certificate.MarshaledCert),
					encodeHashForDatabase(certificate.Hash()),
					certificate.NotValidAfter,
				),
			),
		)
		if err != nil {
			log.Fatalf("failed writing to file %s: %v", fileName, err)
		}
		size += n

		if size > MAX_FILE_SIZE {
			_, err = file.Write([]byte("\nON CONFLICT (certificate_hash) DO NOTHING"))
			if err != nil {
				log.Fatalf("failed writing to file %s: %v", fileName, err)
			}

			err = file.Close()
			if err != nil {
				log.Fatalf("failed closing file %s: %v", fileName, err)
			}

			fileIndex += 1
			fileName := fmt.Sprintf("%s/part-%d.sql", certificatesOutputPath, fileIndex)
			file, err = os.Create(fileName)
			if err != nil {
				log.Fatalf("failed opening file %s: %v", fileName, err)
			}

			size = 0
			isFirstLine = true
		}

		progressBar.Add(1)
	}
	progressBar.Exit()

	_, err = file.Write([]byte("\nON CONFLICT (certificate_hash) DO NOTHING"))
	if err != nil {
		log.Fatalf("failed writing to file %s: %v", fileName, err)
	}

	err = file.Close()
	if err != nil {
		log.Fatalf("failed closing file %s: %v", fileName, err)
	}
}
