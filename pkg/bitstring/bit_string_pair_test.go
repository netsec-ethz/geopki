package bitstring

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

func TestNewBitStringPairMaxValues(t *testing.T) {
	b, err := NewBitStringPair(C_X, C_Y, C_Z, X_BITS, Y_BITS, Z_BITS)

	if b == nil || err != nil {
		t.Fatalf(`should not throw error: %v`, err)
	}
}

func TestNewBitStringPairBigX(t *testing.T) {
	b, err := NewBitStringPair(C_X+1, C_Y, C_Z, X_BITS, Y_BITS, Z_BITS)

	if b != nil || err == nil {
		t.Fatalf(`should throw, received: %v`, err)
	}
}

func TestNewBitStringPairBigY(t *testing.T) {
	b, err := NewBitStringPair(C_X, C_Y+1, C_Z, X_BITS, Y_BITS, Z_BITS)

	if b != nil || err == nil {
		t.Fatalf(`should throw, received: %v`, err)
	}
}

func TestNewBitStringPairBigZ(t *testing.T) {
	b, err := NewBitStringPair(C_X, C_Y, C_Z+1, X_BITS, Y_BITS, Z_BITS)

	if b != nil || err == nil {
		t.Fatalf(`should throw, received: %v`, err)
	}
}

func TestNewBitStringPairBigXPrecision(t *testing.T) {
	b, err := NewBitStringPair(C_X, C_Y, C_Z, X_BITS+1, Y_BITS, Z_BITS)

	if b != nil || err == nil {
		t.Fatalf(`should throw, received: %v`, err)
	}
}

func TestNewBitStringPairBigYPrecision(t *testing.T) {
	b, err := NewBitStringPair(C_X, C_Y, C_Z, X_BITS, Y_BITS+1, Z_BITS)

	if b != nil || err == nil {
		t.Fatalf(`should throw, received: %v`, err)
	}
}

func TestNewBitStringPairBigZPrecision(t *testing.T) {
	b, err := NewBitStringPair(C_X, C_Y, C_Z, X_BITS, Y_BITS, Z_BITS+1)

	if b != nil || err == nil {
		t.Fatalf(`should throw, received: %v`, err)
	}
}

// ensure (xPrecision == yPrecision || xPrecision == yPrecision+1)
func TestNewBitStringPairInvalidXYPrecision(t *testing.T) {
	b, err := NewBitStringPair(0, 0, C_Z, 0, 1, Z_BITS)

	if b != nil || err == nil {
		t.Fatalf(`should throw, received: %v`, err)
	}

	b, err = NewBitStringPair(0, 0, C_Z, 0, 2, Z_BITS)

	if b != nil || err == nil {
		t.Fatalf(`should throw, received: %v`, err)
	}

	b, err = NewBitStringPair(0, 0, C_Z, 1, 2, Z_BITS)

	if b != nil || err == nil {
		t.Fatalf(`should throw, received: %v`, err)
	}
}

func TestNewBitStringPairInvalidXPrecision(t *testing.T) {
	b, err := NewBitStringPair(32, 0, C_Z, 5, 5, Z_BITS)

	if b != nil || err == nil {
		t.Fatalf(`should throw, received: %v`, err)
	}
}

func TestNewBitStringPairInvalidYPrecision(t *testing.T) {
	b, err := NewBitStringPair(0, 32, C_Z, 5, 5, Z_BITS)

	if b != nil || err == nil {
		t.Fatalf(`should throw, received: %v`, err)
	}
}

func TestNewBitStringPairInvalidZPrecision(t *testing.T) {
	b, err := NewBitStringPair(C_X, C_Y, 16, X_BITS, Y_BITS, 4)

	if b != nil || err == nil {
		t.Fatalf(`should throw, received: %v`, err)
	}
}

func TestBitStringPairFromStringPair(t *testing.T) {
	b, err := BitStringPairFromStringPair("001", "0110")

	if b == nil || err != nil {
		t.Fatalf(`should not throw error: %v`, err)
	}

	// 2^24
	if b.XMin != 16777216 {
		t.Fatalf(`invalid xMin: %d`, b.XMin)
	}

	if b.XPrecision != 2 {
		t.Fatalf(`invalid xPrecision: %d`, b.XPrecision)
	}

	if b.YMin != 0 {
		t.Fatalf(`invalid yMin: %d`, b.YMin)
	}

	if b.YPrecision != 1 {
		t.Fatalf(`invalid yPrecision: %d`, b.YPrecision)
	}

	// 2^13 + 2^12
	if b.ZMin != 12288 {
		t.Fatalf(`invalid zMin: %d`, b.ZMin)
	}

	if b.ZPrecision != 4 {
		t.Fatalf(`invalid zPrecision: %d`, b.ZPrecision)
	}
}

func TestBitStringPairFromStringPairLongXY(t *testing.T) {
	b, err := BitStringPairFromStringPair("001", strings.Repeat("1", int(Z_BITS+1)))

	if b != nil || err == nil {
		t.Fatalf(`should throw error`)
	}
}

func TestBitStringPairFromStringPairLongZ(t *testing.T) {
	b, err := BitStringPairFromStringPair(strings.Repeat("1", int(XY_BITS+1)), "0110")

	if b != nil || err == nil {
		t.Fatalf(`should throw error`)
	}
}

func TestBitStringPairFromStringPaiInvalidX(t *testing.T) {
	b, err := BitStringPairFromStringPair("a01", "0110")

	if b != nil || err == nil {
		t.Fatalf(`should throw error`)
	}
}

func TestBitStringPairFromStringPaiInvalidY(t *testing.T) {
	b, err := BitStringPairFromStringPair("0a1", "0110")

	if b != nil || err == nil {
		t.Fatalf(`should throw error`)
	}
}

func TestBitStringPairFromStringPaiInvalidZ(t *testing.T) {
	b, err := BitStringPairFromStringPair("001", "011a")

	if b != nil || err == nil {
		t.Fatalf(`should throw error`)
	}
}

func TestXYBitStringFromGeodeticCoordinates(t *testing.T) {
	b, err := XYBitStringFromGeodeticCoordinates(5.31972, 60.39047)

	if b == nil || err != nil {
		t.Fatalf(`should not throw error: %v`, err)
	}

	if b.XMin != 34546099 {
		t.Fatalf(`invalid xMin: %d`, b.XMin)
	}
	if b.YMin != 28034815 {
		t.Fatalf(`invalid yMin: %d`, b.YMin)
	}

	// should always be the full precision for points
	if b.XPrecision != X_BITS {
		t.Fatalf(`invalid xPrecision: %d`, b.XPrecision)
	}
	if b.YPrecision != Y_BITS {
		t.Fatalf(`invalid yPrecision: %d`, b.YPrecision)
	}
}

func TestXYBitStringFromGeodeticCoordinatesInvalidLongitude(t *testing.T) {
	b, err := XYBitStringFromGeodeticCoordinates(-181, 60.39047)

	if b != nil || err == nil {
		t.Fatalf(`should throw error`)
	}

	b, err = XYBitStringFromGeodeticCoordinates(180.3, 60.39047)

	if b != nil || err == nil {
		t.Fatalf(`should throw error`)
	}
}

func TestXYBitStringFromGeodeticCoordinatesInvalidLatitude(t *testing.T) {
	b, err := XYBitStringFromGeodeticCoordinates(5.31972, 90.5)

	if b != nil || err == nil {
		t.Fatalf(`should throw error`)
	}

	b, err = XYBitStringFromGeodeticCoordinates(5.31972, -91)

	if b != nil || err == nil {
		t.Fatalf(`should throw error`)
	}
}

func TestZBitStringFromGeodeticCoordinate(t *testing.T) {
	b, err := ZBitStringFromGeodeticCoordinate(1337)

	if b == nil || err != nil {
		t.Fatalf(`should not throw error: %v`, err)
	}

	if int16(b.ZMin) != 1337-D {
		t.Fatalf(`invalid zMin: %d`, b.ZMin)
	}
	if b.ZPrecision != Z_BITS {
		t.Fatalf(`invalid zPrecision: %d`, b.ZPrecision)
	}
}

func TestZBitStringFromGeodeticCoordinateInvalidAltitude(t *testing.T) {
	b, err := ZBitStringFromGeodeticCoordinate(-20000)

	if b != nil || err == nil {
		t.Fatalf(`should throw error`)
	}

	b, err = ZBitStringFromGeodeticCoordinate(50000)

	if b != nil || err == nil {
		t.Fatalf(`should throw error`)
	}
}

func TestBitStringPairFromGeodeticCoordinates(t *testing.T) {
	b, err := BitStringPairFromGeodeticCoordinates(5.31972, 60.39047, 1337)

	if b == nil || err != nil {
		t.Fatalf(`should not throw error: %v`, err)
	}

	if b.XMin != 34546099 {
		t.Fatalf(`invalid xMin: %d`, b.XMin)
	}
	if b.YMin != 28034815 {
		t.Fatalf(`invalid yMin: %d`, b.YMin)
	}
	if int16(b.ZMin) != 1337-D {
		t.Fatalf(`invalid zMin: %d`, b.ZMin)
	}

	// should always be the full precision for points
	if b.XPrecision != X_BITS {
		t.Fatalf(`invalid xPrecision: %d`, b.XPrecision)
	}
	if b.YPrecision != Y_BITS {
		t.Fatalf(`invalid yPrecision: %d`, b.YPrecision)
	}

	if b.ZPrecision != Z_BITS {
		t.Fatalf(`invalid zPrecision: %d`, b.ZPrecision)
	}
}

func TestXBitString(t *testing.T) {
	b, _ := BitStringPairFromStringPair("010011010001", "011")

	if b.XBitString() != "001000" {
		t.Fatalf(`invalid XBitString: %s`, b.XBitString())
	}
}

func TestYBitString(t *testing.T) {
	b, _ := BitStringPairFromStringPair("010011010001", "011")

	if b.YBitString() != "101101" {
		t.Fatalf(`invalid YBitString: %s`, b.YBitString())
	}
}

func TestZBitString(t *testing.T) {
	b, _ := BitStringPairFromStringPair("010011010001", "111001")

	if b.ZBitString.String() != "111001" {
		t.Fatalf(`invalid ZBitString: %s`, b.ZBitString.String())
	}
}

func TestXMax(t *testing.T) {
	b, _ := BitStringPairFromStringPair("010011010001", "111001")

	if b.XMax() != 9437184 {
		t.Fatalf(`invalid XMax(): %d`, b.XMax())
	}
}

func TestYMax(t *testing.T) {
	b, _ := BitStringPairFromStringPair("010011010001", "111001")

	if b.YMax() != 24117248 {
		t.Fatalf(`invalid YMax(): %d`, b.YMax())
	}
}

func TestZMax(t *testing.T) {
	b, _ := BitStringPairFromStringPair("010011010001", "111001")

	if b.ZMax() != 29696 {
		t.Fatalf(`invalid ZMax(): %d`, b.YMax())
	}
}

func TestBitStringPair(t *testing.T) {
	xyBitString := "010011010001"
	zBitString := "111001"

	b, _ := BitStringPairFromStringPair(xyBitString, zBitString)
	xy, z := b.BitStringPair()

	if xy != xyBitString {
		t.Fatalf(`invalid xy bit string: %s`, xy)
	}

	if z != zBitString {
		t.Fatalf(`invalid z bitstring: %s`, z)
	}
}

func TestRawXYBitStringPair(t *testing.T) {
	xyBitString := "010011010001"
	zBitString := "111001"

	b, _ := BitStringPairFromStringPair(xyBitString, zBitString)
	p := b.RawBitStringPair()

	if int(p.RawXYBitString.XYBitStringLen) != len(xyBitString) {
		t.Fatalf(`invalid xy bit string len, received %d, expected %d`, p.RawXYBitString.XYBitStringLen, len(xyBitString))
	}

	if int(p.RawZBitString.ZBitStringLen) != len(zBitString) {
		t.Fatalf(`invalid z bit string len, received %d, expected %d`, p.RawZBitString.ZBitStringLen, len(zBitString))
	}

	paddedXYBitString := xyBitString + strings.Repeat("0", 64-len(xyBitString))
	paddedZBitString := zBitString + strings.Repeat("0", 16-len(zBitString))

	parsedXYBitString, err := strconv.ParseUint(paddedXYBitString, 2, 64)
	if err != nil {
		t.Fatalf(`should not throw error: %v`, err)
	}
	parsedZBitString, err := strconv.ParseUint(paddedZBitString, 2, 16)
	if err != nil {
		t.Fatalf(`should not throw error: %v`, err)
	}

	if parsedXYBitString != p.RawXYBitString.XYBitString {
		t.Fatalf(`Invalid bit string, expected %d, received %d`, p.RawXYBitString.XYBitString, parsedXYBitString)
	}

	if uint16(parsedZBitString) != p.RawZBitString.ZBitString {
		t.Fatalf(`Invalid bit string, expected %d, received %d`, p.RawZBitString.ZBitString, parsedZBitString)
	}
}

const EPSILON = 0.001

func isEpsilonClose(value, reference, epsilon float64) bool {
	return math.Abs(value-reference) <= epsilon
}

func TestUndiscretize(t *testing.T) {
	longitude, latitude, altitude := Undiscretize(0, 0, 0)

	if !isEpsilonClose(longitude, -180, EPSILON) {
		t.Fatalf(`Invalid undescretized longitude, expected %d, received %f`, -180, longitude)
	}

	if !isEpsilonClose(latitude, -90, EPSILON) {
		t.Fatalf(`Invalid undescretized longitude, expected %d, received %f`, -90, latitude)
	}

	if !isEpsilonClose(altitude, float64(D), EPSILON) {
		t.Fatalf(`Invalid undescretized altitude, expected %d, received %f`, D, altitude)
	}

	longitude, latitude, altitude = Undiscretize((1 << (X_BITS - 1)), (1 << (Y_BITS - 1)), uint16(-D))

	if !isEpsilonClose(longitude, 0, EPSILON) {
		t.Fatalf(`Invalid undescretized longitude, expected %d, received %f`, 0, longitude)
	}

	if !isEpsilonClose(latitude, 0, EPSILON) {
		t.Fatalf(`Invalid undescretized longitude, expected %d, received %f`, 0, latitude)
	}

	if !isEpsilonClose(altitude, 0, EPSILON) {
		t.Fatalf(`Invalid undescretized altitude, expected %d, received %f`, 0, altitude)
	}

	longitude, latitude, altitude = Undiscretize((1<<X_BITS)-1, (1<<Y_BITS)-1, (1<<Z_BITS)-1)

	if !isEpsilonClose(longitude, 180, EPSILON) {
		t.Fatalf(`Invalid undescretized longitude, expected %d, received %f`, 180, longitude)
	}

	if !isEpsilonClose(latitude, 90, EPSILON) {
		t.Fatalf(`Invalid undescretized longitude, expected %d, received %f`, 90, latitude)
	}

	if !isEpsilonClose(altitude, float64(H), EPSILON) {
		t.Fatalf(`Invalid undescretized altitude, expected %d, received %f`, H, altitude)
	}
}

func TestGeodeticCoordinates(t *testing.T) {
	xyBitString := "010011010001"
	zBitString := "111001"

	b, _ := BitStringPairFromStringPair(xyBitString, zBitString)
	longitude, latitude, altitude := b.GeodeticCoordinates()

	if !isEpsilonClose(longitude, -135, EPSILON) {
		t.Fatalf(`Invalid undescretized longitude, expected %d, received %f`, -135, longitude)
	}

	if !isEpsilonClose(latitude, 36.5625, EPSILON) {
		t.Fatalf(`Invalid undescretized longitude, expected %d, received %f`, 90, latitude)
	}

	// zBitString is a prefix of the z dimension,
	// so ZMin is it right-padded to Z_BITS.
	// Altitudes are offsets from D, the minimum geodetic altitude,
	// so derive the expectation from zBitString and D.
	paddedZBitString := zBitString + strings.Repeat("0", int(Z_BITS)-len(zBitString))
	expectedZMin, err := strconv.ParseUint(paddedZBitString, 2, 16)
	if err != nil {
		t.Fatalf(`should not throw error: %v`, err)
	}

	expectedAltitude := float64(int32(D) + int32(expectedZMin))

	if !isEpsilonClose(altitude, expectedAltitude, EPSILON) {
		t.Fatalf(`Invalid undescretized altitude, expected %f, received %f`, expectedAltitude, altitude)
	}
}
