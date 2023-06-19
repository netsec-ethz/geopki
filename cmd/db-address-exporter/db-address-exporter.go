package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"geopki/pkg/bitstring"
	"geopki/pkg/crypto"
	"io"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"

	goparquet "github.com/fraugster/parquet-go"
	"github.com/schollz/progressbar/v3"
)

const (
	F_GROW                = 1
	MAX_FILE_SIZE         = 300 * 1000 * 1000 // 300 MB
	INSERT_INTO_NODES_STR = "INSERT INTO nodes(bit_string_51,bit_string_51_int,bit_string_15,altitude_min,altitude_max,xy_left_child_hash,xy_right_child_hash,z_left_child_hash,z_right_child_hash,certificate_hashes) VALUES\n"
	INSERT_INTO_CERTS_STR = "INSERT INTO certificates(certificate_hash,certificate,not_valid_after) VALUES\n"
)

type Coordinate struct {
	Longitude float64 `parquet:"lon"`
	Latitude  float64 `parquet:"lat"`
}
type Polygon []Coordinate
type MultiPolygon []Polygon

func (r *MultiPolygon) GeoCertArea() crypto.GeoCertArea {
	interiorRings := make([]crypto.LinearRing, 0)
	exteriorRings := make([]crypto.LinearRing, 0)

	for _, polygon := range *r {
		ring := make([][2]float64, 0)

		for _, coordinate := range polygon {
			ring = append(ring, [2]float64{coordinate.Longitude, coordinate.Latitude})
		}

		interiorRings = append(interiorRings, ring)
	}
	// coordinates := make([2][][][2]float64, 0)

	return crypto.GeoCertArea{
		Type:        "MultiPolygon",
		Coordinates: [2][]crypto.LinearRing{interiorRings, exteriorRings},
	}
}

type ParquetRow struct {
	Domain              string         `parquet:"domain"`
	CertificateId       string         `parquet:"certificate_id"`
	ListOfMultipolygons []MultiPolygon `parquet:"list_of_multipolygons,list"`
	ListOfLevels        []string       `parquet:"list_of_levels"`
	Parents             []string       `parquet:"parents"`
	Children            []string       `parquet:"children"`
	MinBuildingLevel    string         `parquet:"min_building_level"`
	MaxBuildingLevel    string         `parquet:"max_building_level"`
	SurfaceAltitude     float64        `parquet:"surface_geodetic_altitude_aster_30"`
}

func (r *ParquetRow) NotValidAfter() string {
	return "2030-01-01 00:00:00+00"
}

func (r *ParquetRow) Certificate() (*crypto.GeoCertificate, error) {
	listOfAltitudes := make([]([2]float64), len(r.ListOfLevels))
	for i, level := range r.ListOfLevels {
		altitudeBounds, err := levelToAltitude(
			r.MinBuildingLevel,
			r.MaxBuildingLevel,
			level,
			r.SurfaceAltitude,
		)
		if err != nil {
			return nil, fmt.Errorf("failed converting level to altitude: %v", err)
		}

		var bounds [2]float64
		bounds[0] = altitudeBounds.Minimum
		bounds[1] = altitudeBounds.Maximum

		listOfAltitudes[i] = bounds
	}

	areas := make([]crypto.GeoCertArea, len(r.ListOfMultipolygons))
	for i, multipolygon := range r.ListOfMultipolygons {
		areas[i] = multipolygon.GeoCertArea()
	}

	cert := crypto.GeoCertificate{
		CertificateId: r.CertificateId,
		Domain:        r.Domain,
		Areas:         areas,
		AreasAltitude: listOfAltitudes,
		NotValidAfter: r.NotValidAfter(),
	}

	if len(areas) != len(listOfAltitudes) {
		fmt.Printf("%+v\n\n", r)
		return nil, fmt.Errorf("list of areas and altitudes do not have the same length. areas: %d, altitudes: %d, multipolygons: %d, levels: %d", len(areas), len(listOfAltitudes), len(r.ListOfMultipolygons), len(r.ListOfLevels))
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

func unmarshalPolygon(v interface{}) Polygon {
	var slice []Coordinate

	list, ok := v.(map[string]interface{})["list"].([]map[string]interface{})
	if !ok {
		return nil
	}

	for _, it := range list {
		coords := it["item"].(map[string]interface{})
		// lon := coords["lon"].(float64)
		// lat := coords["lat"].(float64)
		lon, err := strconv.ParseFloat(string(coords["lon"].([]byte)), 64)
		if err != nil {
			log.Fatal("?")
		}
		lat, err := strconv.ParseFloat(string(coords["lat"].([]byte)), 64)
		if err != nil {
			log.Fatal("?")
		}

		slice = append(slice, Coordinate{
			Longitude: lon,
			Latitude:  lat,
		})
	}

	return slice
}

func unmarshalMultipolygon(v interface{}) MultiPolygon {
	var slice []Polygon

	list, ok := v.(map[string]interface{})["list"].([]map[string]interface{})
	if !ok {
		return nil
	}

	for _, it := range list {
		slice = append(slice, unmarshalPolygon(it["item"]))
	}

	return slice
}

func unmarshalListOfMultipolygons(v interface{}) []MultiPolygon {
	var slice []MultiPolygon

	list, ok := v.(map[string]interface{})["list"].([]map[string]interface{})
	if !ok {
		return nil
	}

	for _, it := range list {
		slice = append(slice, unmarshalMultipolygon(it["item"]))
	}

	return slice
}

func numpyArrayToStringSlice(v interface{}) []string {
	var slice []string

	list, ok := v.(map[string]interface{})["list"].([]map[string]interface{})
	if !ok {
		return nil
	}

	for _, it := range list {
		item := string(it["item"].([]byte))

		slice = append(slice, item)
	}

	return slice
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

	r, err := os.Open(inputPath)
	if err != nil {
		log.Fatalf("opening file %s failed :%v", inputPath, err)
	}
	defer r.Close()

	fr, err := goparquet.NewFileReader(r)
	if err != nil {
		log.Fatalf("reading file %s failed :%v", inputPath, err)
	}

	// log.Printf("Printing file %s", inputPath)
	// log.Printf("Schema: %s", fr.GetSchemaDefinition())

	certificates := make([]*crypto.GeoCertificate, 0)
	bitstringPairToNode := make(map[bitstring.RawBitStringPair]*crypto.Node)

	progressBar := progressbar.Default(int64(fr.NumRows()), "locate certificates")

	for ; ; progressBar.Add(1) {
		row, err := fr.NextRow()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("reading record failed: %v", err)
		}

		r := new(ParquetRow)

		// set default altitude
		r.SurfaceAltitude = 0

		var x interface{}

		for k, v := range row {
			if k == "certificate_id" {
				r.CertificateId = string(v.([]byte))
			} else if k == "domain" {
				r.Domain = string(v.([]byte))
			} else if k == "list_of_multipolygons" {
				r.ListOfMultipolygons = unmarshalListOfMultipolygons(v)
				x = v
			} else if k == "list_of_levels" {
				r.ListOfLevels = numpyArrayToStringSlice(v)
			} else if k == "parents" {
				r.Parents = numpyArrayToStringSlice(v)
			} else if k == "children" {
				r.Children = numpyArrayToStringSlice(v)
			} else if k == "min_building_level" {
				r.MinBuildingLevel = string(v.([]byte))
			} else if k == "max_building_level" {
				r.MaxBuildingLevel = string(v.([]byte))
			} else if k == "surface_geodetic_altitude_aster_30" {
				r.SurfaceAltitude = v.(float64)
			}

			// if vv, ok := v.([]byte); ok {
			// 	v = string(vv)
			// }
			// log.Printf("\t%s = %v", k, v)
		}
		// if r.Domain == "" {
		// 	continue
		// }

		if r.CertificateId == "rel:1027211;node:4591429319" || len(r.ListOfMultipolygons) > 1 {
			fmt.Printf("%+v\n", x)
		}
		// log.Printf("%v", r)

		certificate, err := r.Certificate()
		if err != nil {
			log.Fatalf("failed converting to a certificate: %v", err)
		}
		certificates = append(certificates, certificate)

		if progressBar.State().CurrentBytes == 1296 {
			log.Fatalf("%s: %d", r.CertificateId, len(r.ListOfMultipolygons[0][0]))
			println("here?")
		}

		bitstringPairs, err := certificate.BitStrings(F_GROW)
		if err != nil {
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
					bitstringPair.XYBitStringLen,
					nil, nil, nil, nil,
					[]crypto.SHA256Hash{certificate.Hash()},
				)
			}

			// iterate over all prefixes of that bit string and add them to bitstringPairToNode
			// first iterate over prefixes of the altitude bit string, including the empty string ''
			for i := uint8(0); i < bitstringPair.RawZBitString.ZBitStringLen; i++ {
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
			for i := uint8(0); i < bitstringPair.RawXYBitString.XYBitStringLen; i++ {
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

	}
	// log.Printf("End of file %s (%d records)", inputPath, count)

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

		xyRightChild, err := bitstring.XYRightChildPair()
		if err == nil {
			xyRightChildNode, ok := bitstringPairToNode[xyRightChild]

			if ok {
				node.SetXYRightChildHash(xyRightChildNode.Hash())
			}
		}

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
	progressBar.Finish()

	// write output
	fileIndex := 0
	fileName := fmt.Sprintf("%s/part-%d.sql", nodesOutputPath, fileIndex)
	size := 0
	isFirstLine := true

	file, err := os.Open(fileName)
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
			file, err = os.Open(fileName)
			if err != nil {
				log.Fatalf("failed opening file %s: %v", fileName, err)
			}

			size = 0
			isFirstLine = true
		}

		progressBar.Add(1)
	}
	progressBar.Finish()

	err = file.Close()
	if err != nil {
		log.Fatalf("failed closing file %s: %v", fileName, err)
	}

	fileIndex = 0
	fileName = fmt.Sprintf("%s/part-%d.sql", certificatesOutputPath, fileIndex)
	size = 0
	isFirstLine = true
	file, err = os.Open(fileName)
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
			err = file.Close()
			if err != nil {
				log.Fatalf("failed closing file %s: %v", fileName, err)
			}

			fileIndex += 1
			fileName := fmt.Sprintf("%s/part-%d.sql", nodesOutputPath, fileIndex)
			file, err = os.Open(fileName)
			if err != nil {
				log.Fatalf("failed opening file %s: %v", fileName, err)
			}

			size = 0
			isFirstLine = true
		}

		progressBar.Add(1)
	}
	progressBar.Finish()

	err = file.Close()
	if err != nil {
		log.Fatalf("failed closing file %s: %v", fileName, err)
	}
}
