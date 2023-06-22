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
	geom := BitStringToGdalGeometry(xyBitstring)
	r := g.Geometry.Intersects(*geom)

	geom.Destroy()
	return r
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

	initialGeometry := BitStringToGdalGeometry(initialBitString)
	currentArea := initialGeometry.Area()
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
	if area.Type != "MultiPolygon" {
		return nil, fmt.Errorf("area.Type must be equal to 'MultiPolygon'")
	}

	WGS84 := getWGS84()
	exteriorRing := area.Coordinates[0]

	geometries := make([]bitstring.Geometry2D, len(exteriorRing))
	for i, polygon := range exteriorRing {

		wkt := "POLYGON (("
		// array of ccw order points of the loop
		for i, pt := range polygon {
			if i > 0 {
				wkt += ","
			}
			wkt += fmt.Sprintf("%f %f", pt[0], pt[1])
		}
		wkt += "))"

		geometry, err := gdal.CreateFromWKT(wkt, WGS84)
		if err != nil {
			println(wkt)
			return nil, err
		}

		geometry.SetSpatialReference(WGS84)

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

		bs, err := bitstring.ExtrudedPolygonsToBitStringPairs(geometries, altitudeMin, altitudeMax, fGrow)
		if err != nil {
			return nil, err
		}

		for _, g := range geometries {
			g.Destroy()
		}

		bitstrings = append(bitstrings, bs...)
	}

	return bitstrings, nil
}
