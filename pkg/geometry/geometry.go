package geometry

import (
	"fmt"
	"geopki/pkg/bitstring"
	"geopki/pkg/crypto"
	"log"
	"math"

	"github.com/golang/geo/s2"
	"github.com/lukeroth/gdal"
)

// the gdal implementation of the 'Geometry2D' interface
type GdalGeometry2D struct {
	Geometry *gdal.Geometry
}

func (g *GdalGeometry2D) Intersects(xyBitstring *bitstring.XYBitString) bool {
	return g.Geometry.Intersects(*LoopToGdalGeometry(xyBitstring.Loop()))
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
	currentArea := LoopToGdalGeometry(initialBitString.Loop()).Area()

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

type GdalCircleApproximator struct{}

func (s *GdalCircleApproximator) ApproximateCircle(longitude, latitude float64, radiusM uint8) bitstring.Geometry2D {
	sphere := bitstring.ApproximateCircle(
		longitude,
		latitude,
		radiusM,
		// use 64-sided polygon
		16,
	)

	return &GdalGeometry2D{
		Geometry: LoopToGdalGeometry(sphere),
	}
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

func LoopToGdalGeometry(loop *s2.Loop) *gdal.Geometry {
	g := gdal.CreateFromJson(bitstring.LoopToGeoPolygon(loop))
	g.SetSpatialReference(getWGS84())

	return &g
}

func BitStringToGdalGeometry(bitString *bitstring.XYBitString) *gdal.Geometry {
	return LoopToGdalGeometry(bitString.Loop())
}

func CertificateToGeometries(area *crypto.GeoCertArea) ([]bitstring.Geometry2D, error) {
	loops, err := area.Loops()
	if err != nil {
		return nil, err
	}

	geometries := make([]bitstring.Geometry2D, len(loops))
	for i, loop := range loops {
		geometries[i] = &GdalGeometry2D{
			Geometry: LoopToGdalGeometry(loop),
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

		bs, err := bitstring.ExtrudedPolygonsToBitStringPairs(geometries, altitudeMin, altitudeMax, fGrow)
		if err != nil {
			return nil, err
		}

		bitstrings = append(bitstrings, bs...)
	}

	return bitstrings, nil
}
