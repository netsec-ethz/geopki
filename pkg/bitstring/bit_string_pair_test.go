package bitstring

import (
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
	if b.xMin != 16777216 {
		t.Fatalf(`invalid xMin: %d`, b.xMin)
	}

	if b.xPrecision != 2 {
		t.Fatalf(`invalid xPrecision: %d`, b.xPrecision)
	}

	if b.yMin != 0 {
		t.Fatalf(`invalid yMin: %d`, b.yMin)
	}

	if b.yPrecision != 1 {
		t.Fatalf(`invalid yPrecision: %d`, b.yPrecision)
	}

	// 2^13 + 2^12
	if b.zMin != 12288 {
		t.Fatalf(`invalid zMin: %d`, b.zMin)
	}

	if b.zPrecision != 4 {
		t.Fatalf(`invalid zPrecision: %d`, b.zPrecision)
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

	if b.xMin != 34546099 {
		t.Fatalf(`invalid xMin: %d`, b.xMin)
	}
	if b.yMin != 28034814 {
		t.Fatalf(`invalid yMin: %d`, b.yMin)
	}

	// should always be the full precision for points
	if b.xPrecision != X_BITS {
		t.Fatalf(`invalid xPrecision: %d`, b.xPrecision)
	}
	if b.yPrecision != Y_BITS {
		t.Fatalf(`invalid yPrecision: %d`, b.yPrecision)
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

	if int16(b.zMin) != 1337-D {
		t.Fatalf(`invalid zMin: %d`, b.zMin)
	}
	if b.zPrecision != Z_BITS {
		t.Fatalf(`invalid zPrecision: %d`, b.zPrecision)
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

	if b.xMin != 34546099 {
		t.Fatalf(`invalid xMin: %d`, b.xMin)
	}
	if b.yMin != 28034814 {
		t.Fatalf(`invalid yMin: %d`, b.yMin)
	}
	if int16(b.zMin) != 1337-D {
		t.Fatalf(`invalid zMin: %d`, b.zMin)
	}

	// should always be the full precision for points
	if b.xPrecision != X_BITS {
		t.Fatalf(`invalid xPrecision: %d`, b.xPrecision)
	}
	if b.yPrecision != Y_BITS {
		t.Fatalf(`invalid yPrecision: %d`, b.yPrecision)
	}

	if b.zPrecision != Z_BITS {
		t.Fatalf(`invalid zPrecision: %d`, b.zPrecision)
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

	if b.ZBitString.BitString() != "111001" {
		t.Fatalf(`invalid ZBitString: %s`, b.ZBitString.BitString())
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
