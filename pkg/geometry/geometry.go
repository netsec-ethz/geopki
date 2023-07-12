package geometry

import (
	"fmt"
	"geopki/pkg/bitstring"
	"geopki/pkg/crypto"
	"log"
	"math"
	"strings"
	"sync"

	"github.com/golang/geo/s2"
	"github.com/lukeroth/gdal"
)

var WGS84 = getWGS84()

// the gdal implementation of the 'Geometry2D' interface
type GdalGeometry2D struct {
	Geometry *gdal.Geometry
}

func (g *GdalGeometry2D) Intersects(xyBitstring *bitstring.XYBitString) (bool, error) {
	geom, err := BitStringToGdalGeometry(xyBitstring)
	if err != nil {
		return false, err
	}

	r := g.Geometry.Intersects(*geom)

	// free memory
	geom.Destroy()

	return r, nil
}

func (g *GdalGeometry2D) InitialXYBitString(fGrow float64) (*bitstring.XYBitString, error) {
	polygon := g.Geometry

	exteriorRing := polygon.Geometry(0)
	longitude, latitude, _ := exteriorRing.Point(0)

	initialBitString, err := bitstring.XYBitStringFromGeodeticCoordinates(
		longitude,
		latitude,
	)
	if err != nil {
		return nil, err
	}

	// grow initial bitstring to 'maxArea'
	maxArea := polygon.Area() * fGrow

	initialGeometry, err := BitStringToGdalGeometry(initialBitString)
	if err != nil {
		return nil, fmt.Errorf("could not convert bit string to gdal geometry: %v", err)
	}

	currentArea := initialGeometry.Area()

	// free memory
	initialGeometry.Destroy()

	growSteps := math.Log2(maxArea / currentArea)

	if growSteps < 0 {
		// polygon is smaller than the smallest bit string are -> do nothing
	} else if growSteps > float64(initialBitString.XPrecision+initialBitString.YPrecision) {
		return nil, fmt.Errorf("something seems off, cannot grow larger than the whole world (%f / %f = %f > %d;)", maxArea, currentArea, growSteps, initialBitString.XPrecision+initialBitString.YPrecision)
	} else {
		err = initialBitString.Grow2D(uint8(growSteps))
		if err != nil {
			return nil, err
		}
	}

	return initialBitString, nil
}

func (g *GdalGeometry2D) Destroy() {
	g.Geometry.Destroy()
}

type GdalCircleApproximator struct{}

func (s *GdalCircleApproximator) ApproximateCircle(longitude, latitude float64, radiusM uint8) (bitstring.Geometry2D, error) {
	sphere := bitstring.ApproximateCircle(
		longitude,
		latitude,
		radiusM,
		// use 64-sided polygon
		16,
	)

	g, err := LoopToGdalGeometry(sphere)
	if err != nil {
		return nil, fmt.Errorf("could not convert bit string to gdal geometry: %v", err)
	}

	return &GdalGeometry2D{
		Geometry: g,
	}, nil
}

// https://epsg.io/4326
func getWGS84() gdal.SpatialReference {
	WGS84 := gdal.CreateSpatialReference("")
	err := WGS84.FromEPSG(4326)
	if err != nil {
		log.Fatalf("invalid epsg code for WGS84: %v", err)
	}

	return WGS84
}

var stringBuilderPool = sync.Pool{
	New: func() any {
		// The Pool's New function should generally only return pointer
		// types, since a pointer can be put into the return interface
		// value without an allocation:
		return new(strings.Builder)
	},
}

func LoopToGdalGeometry(loop *s2.Loop) (*gdal.Geometry, error) {

	vertices := loop.Vertices()

	wkt := stringBuilderPool.Get().(*strings.Builder)
	defer func() {
		wkt.Reset()
		stringBuilderPool.Put(wkt)
	}()

	wkt.WriteString("POLYGON ((")
	// array of ccw order points of the loop
	for i, vertex := range vertices {
		coordinate := s2.LatLngFromPoint(vertex)

		if i > 0 {
			wkt.WriteString(",")
		}
		wkt.WriteString(fmt.Sprintf("%f %f", coordinate.Lng.Degrees(), coordinate.Lat.Degrees()))
	}

	// end with coordinates of first point
	coordinate := s2.LatLngFromPoint(vertices[0])
	wkt.WriteString(fmt.Sprintf(",%f %f))", coordinate.Lng.Degrees(), coordinate.Lat.Degrees()))

	geometry, err := gdal.CreateFromWKT(wkt.String(), WGS84)
	if err != nil {
		println(wkt.String())
		return nil, err
	}

	return &geometry, nil
}

func BitStringToGdalGeometry(bitString *bitstring.XYBitString) (*gdal.Geometry, error) {
	return LoopToGdalGeometry(bitString.Loop())
}

func CertificateToGeometries(area *crypto.GeoCertArea) ([]bitstring.Geometry2D, error) {
	if area.Type != "MultiPolygon" {
		return nil, fmt.Errorf("area.Type must be equal to 'MultiPolygon'")
	}

	exteriorRing := area.Coordinates[0]

	geometries := make([]bitstring.Geometry2D, len(exteriorRing))
	for i, polygon := range exteriorRing {

		wkt := stringBuilderPool.Get().(*strings.Builder)
		defer func() {
			wkt.Reset()
			stringBuilderPool.Put(wkt)
		}()

		wkt.WriteString("POLYGON ((")
		// array of ccw order points of the loop
		for i, pt := range polygon {
			if i > 0 {
				wkt.WriteString(",")
			}
			wkt.WriteString(fmt.Sprintf("%f %f", pt[0], pt[1]))
		}
		wkt.WriteString("))")

		geometry, err := gdal.CreateFromWKT(wkt.String(), WGS84)
		if err != nil {
			println(wkt.String())
			return nil, err
		}

		geometries[i] = &GdalGeometry2D{
			Geometry: &geometry,
		}
	}

	return geometries, nil
}

// defined in geometry since it uses gdal
func CertificateToBitStrings(cert *crypto.GeoCertificate, fGrow float64) ([]*bitstring.RawBitStringPair, error) {
	bitstrings := make([]*bitstring.RawBitStringPair, 0)

	for i, area := range cert.Areas {
		altitude := cert.AreasAltitude[i]
		altitudeMin := altitude[0]
		altitudeMax := altitude[1]

		geometries, err := CertificateToGeometries(&area)
		if err != nil {
			return nil, err
		}

		defer func() {
			// free memory
			for _, g := range geometries {
				g.Destroy()
			}
		}()

		bs, err := bitstring.ExtrudedPolygonsToBitStringPairs(geometries, altitudeMin, altitudeMax, fGrow)
		if err != nil {
			return nil, err
		}

		bitstrings = append(bitstrings, bs...)
	}

	return bitstrings, nil
}
