package bitstring

import (
	"fmt"
	"math"
	"strconv"
)

const (
	// The number of bits in used in the discretization of the `x` dimension.
	X_BITS uint8 = 26

	// The number of bits in used in the discretization of the `y` dimension.
	Y_BITS uint8 = 25

	// The number of x and y bits, maximum length of the xy bit string
	XY_BITS = X_BITS + Y_BITS

	// The number of bits in used in the discretization of the `z` dimension.
	// Also the maximum length of z bit strings
	Z_BITS uint8 = 15

	// The maximum value of the discretized `x` coordinate.
	C_X uint32 = (1 << X_BITS) - 1

	// The maximum value of the discretized `y` coordinate.
	C_Y uint32 = (1 << Y_BITS) - 1

	// The maximum value of the discretized `y` coordinate.
	C_Z uint16 = (1 << Z_BITS) - 1

	// The minimum geodetic altitude in meters
	D int16 = -10000

	// The minimum geodetic altitude in meters
	H int16 = int16(C_Z) + int16(D)
)

// this struct is analogous to the 'DiscretizedVoxel' class in '../coordinatez.py'
type BitStringPair struct {
	// The smallest x, y and z coordinates of the voxel.
	// Must be in `[0, C_X]`, `[0, C_Y]` or `[0, C_Z]` respectively.
	xMin, yMin uint32
	zMin       uint16

	// The number of bits used to encode `xMin`, `yMin` and `zMin`, respectively.
	// Must be in `[0, X_BITS]`, `[0, Y_BITS]` or `[0, Z_BITS]`, respectively.
	xPrecision, yPrecision, zPrecision uint8
}

// Creates a new bit string pair instance and ensures the passed values represent
// a valid bit string pair.
func NewBitStringPair(
	xMin, yMin uint32,
	zMin uint16,
	xPrecision, yPrecision, zPrecision uint8,
) (*BitStringPair, error) {

	// ensure xMin, yMin and zMin are in their respective ranges
	if xMin >= C_X {
		return nil, fmt.Errorf(
			"xMin (%d) is greater than C_X (%d)",
			xMin,
			C_X,
		)
	}

	if yMin >= C_Y {
		return nil, fmt.Errorf(
			"yMin (%d) is greater than C_Y (%d)",
			yMin,
			C_Y,
		)
	}

	if zMin >= C_Z {
		return nil, fmt.Errorf(
			"zMin (%d) is greater than C_Z (%d)",
			zMin,
			C_Z,
		)
	}

	// ensure xPrecision, yPrecision and zPrecision are in their respective ranges
	if xPrecision >= X_BITS {
		return nil, fmt.Errorf(
			"xPrecision (%d) is greater than X_BITS (%d)",
			xPrecision,
			X_BITS,
		)
	}

	if yPrecision >= Y_BITS {
		return nil, fmt.Errorf(
			"yPrecision (%d) is greater than Y_BITS (%d)",
			yPrecision,
			Y_BITS,
		)
	}

	if zPrecision >= Z_BITS {
		return nil, fmt.Errorf(
			"zPrecision (%d) is greater than Z_BITS (%d)",
			zPrecision,
			Z_BITS,
		)
	}

	// ensure there can be a proper interleaving of the x and y bit strings
	if !(xPrecision == yPrecision || xPrecision == yPrecision+1) {
		return nil, fmt.Errorf(
			"the x and y precisions must either match or the x precision must be exactly one greater to allow a bit interleaving. xPrecision=%d, yPrecisoin=%d given",
			xPrecision,
			yPrecision,
		)
	}

	// ensure that 'xPrecision' are enough bits to encode 'xMin'.
	// get a bit string of all ones, shift it (X_BITS - xPrecision) to the left
	// and perform a bitwise AND with xMin. This clears all but the first xPrecision
	// bits. Should xPrecision be enough bits, the value should stay the same

	var xMask uint32 = math.MaxUint32 << (X_BITS - xPrecision)
	var yMask uint32 = math.MaxUint32 << (Y_BITS - yPrecision)
	var zMask uint16 = math.MaxUint16 << (Z_BITS - zPrecision)

	if xMin != (xMin & xMask) {
		return nil, fmt.Errorf(
			"the given precision of %d bits to do suffice to encode the x coordinate %d",
			xPrecision,
			xMin,
		)
	}

	if yMin != (yMin & yMask) {
		return nil, fmt.Errorf(
			"the given precision of %d bits to do suffice to encode the y coordinate %d",
			yPrecision,
			yMin,
		)
	}

	if zMin != (zMin & zMask) {
		return nil, fmt.Errorf(
			"the given precision of %d bits to do suffice to encode the z coordinate %d",
			zPrecision,
			zMin,
		)
	}

	pair := &BitStringPair{
		xMin:       xMin,
		yMin:       yMin,
		zMin:       zMin,
		xPrecision: xPrecision,
		yPrecision: yPrecision,
		zPrecision: zPrecision,
	}

	return pair, nil
}

// Creates an instance from a given bit string encoding the x and y coordinate and
// a second bit string encoding the z coordinate.
func BitStringPairFromStringPair(
	xyBitString, zBitString string,
) (*BitStringPair, error) {

	if len(xyBitString) > int(XY_BITS) {
		return nil, fmt.Errorf(
			"the given xy bit string has a length of %d but should be at most %d",
			len(xyBitString),
			XY_BITS,
		)
	}

	if len(zBitString) > int(Z_BITS) {
		return nil, fmt.Errorf(
			"the given z bit string has a length of %d but should be at most %d",
			len(zBitString),
			Z_BITS,
		)
	}

	// assuming len(XYBitString) is odd, is equivalent to rounding up,
	// if it is even, the additional .5 will be truncated by uint8(...)
	// this works because the bit string starts with a x bit meaning
	// either xPrecision == YPrecision or xPrecision == YPrecision + 1
	var xPrecision uint8 = uint8((len(xyBitString) + 1) / 2)
	var yPrecision uint8 = uint8(len(xyBitString) / 2)

	// de-interleave the xy bit string
	var xBitString = make([]rune, xPrecision)
	var yBitString = make([]rune, yPrecision)

	for i, c := range xyBitString {
		if i%2 == 0 {
			xBitString[i/2] = c
		} else {
			yBitString[i/2] = c
		}
	}

	// interpret the bit strings as integers
	x, err := strconv.ParseUint(string(xBitString), 2, int(xPrecision))
	if err != nil {
		return nil, err
	}

	y, err := strconv.ParseUint(string(yBitString), 2, int(yPrecision))
	if err != nil {
		return nil, err
	}

	z, err := strconv.ParseUint(zBitString, 2, int(Z_BITS))
	if err != nil {
		return nil, err
	}

	pair := &BitStringPair{
		xMin:       uint32(x),
		yMin:       uint32(y),
		zMin:       uint16(z),
		xPrecision: xPrecision,
		yPrecision: yPrecision,
		zPrecision: uint8(len(zBitString)),
	}

	return pair, nil
}

// Creates a BitStringPair instance from a geodetic coordinate
func BitStringPairFromGeodeticCoordinates(
	longitude, latitude, altitude float64,
) (*BitStringPair, error) {

	// ensure the passed longitude, latitude and altitude values are valid
	if longitude < -180 || longitude > 180 {
		return nil, fmt.Errorf(
			"longitudes must be in the range [-180, 180], %f given",
			longitude,
		)
	}

	if latitude < -90 || latitude > 90 {
		return nil, fmt.Errorf(
			"latitudes must be in the range [-90, 90], %f given",
			latitude,
		)
	}

	if altitude < float64(D) || altitude > float64(H) {
		return nil, fmt.Errorf(
			"altitudes must be in the range [%d, %d], %f given",
			D,
			H,
			altitude,
		)
	}

	var x uint32 = uint32(((longitude + 180) / 360) * float64(C_X))
	var y uint32 = uint32(((latitude + 90) / 180) * float64(C_Y))
	var z uint16 = uint16(altitude - float64(D))

	// create an instance with the full precision
	pair := &BitStringPair{
		xMin:       x,
		yMin:       y,
		zMin:       z,
		xPrecision: X_BITS,
		yPrecision: Y_BITS,
		zPrecision: Z_BITS,
	}

	return pair, nil
}

// The bit string encoding the `xMin` value, i.e. the `xPrecision` MSBs
func (pair BitStringPair) GetXBitString() string {
	return fmt.Sprintf(
		// left-pad with 0s to X_BITS
		"%0*s",
		X_BITS,
		// convert integer to bit string
		strconv.FormatInt(int64(pair.xMin), 2),
		// only use the first xPrecision (most significant) bits
	)[:pair.xPrecision]
}

// The bit string encoding the `yMin` value, i.e. the `yPrecision` MSBs
func (pair BitStringPair) GetYBitString() string {
	return fmt.Sprintf(
		// left-pad with 0s to Y_BITS
		"%0*s",
		Y_BITS,
		// convert integer to bit string
		strconv.FormatInt(int64(pair.yMin), 2),
		// only use the first yPrecision (most significant) bits
	)[:pair.yPrecision]
}

// The bit string encoding the `zMin` value, i.e. the `zPrecision` MSBs
func (pair BitStringPair) GetZBitString() string {
	return fmt.Sprintf(
		// left-pad with 0s to Z_BITS
		"%0*s",
		Z_BITS,
		// convert integer to bit string
		strconv.FormatInt(int64(pair.zMin), 2),
		// only use the first zPrecision (most significant) bits
	)[:pair.zPrecision]
}

// The smallest discretized `x` coordinate that is no longer in the voxel
func (pair BitStringPair) GetXMax() uint32 {
	// conceptually set the X_BITS - xPrecision least significant bits to 1
	// and add one to the resulting integer

	return pair.xMin + (1 << (X_BITS - pair.xPrecision))
}

// The smallest discretized `y` coordinate that is no longer in the voxel
func (pair BitStringPair) GetYMax() uint32 {
	// conceptually set the Y_BITS - yPrecision least significant bits to 1
	// and add one to the resulting integer

	return pair.yMin + (1 << (Y_BITS - pair.yPrecision))
}

// The smallest discretized `z` coordinate that is no longer in the voxel
func (pair BitStringPair) GetZMax() uint16 {
	// conceptually set the Z_BITS - zPrecision least significant bits to 1
	// and add one to the resulting integer

	return pair.zMin + (1 << (Z_BITS - pair.zPrecision))
}

// Returns a string representation of the discretized coordinates
func (pair BitStringPair) String() string {
	return "" +
		fmt.Sprintf("X: [%d, %d)", pair.xMin, pair.GetXMax()) + "\n" +
		fmt.Sprintf("Y: [%d, %d)", pair.yMin, pair.GetYMax()) + "\n" +
		fmt.Sprintf("Z: [%d, %d)", pair.zMin, pair.GetZMax())
}

// Returns the bit string representation of the `x` and `y` coordinate and the
// bit representation of the `z` coordinate.
func (pair BitStringPair) BitStringPair() (string, string) {
	xBitString := pair.GetXBitString()
	yBitString := pair.GetYBitString()
	zBitString := pair.GetZBitString()

	// interleave XBitString and YBitString
	var xyBitString = make([]rune, len(xBitString)+len(yBitString))
	// all even values come from the x coordiate
	for i, c := range xBitString {
		xyBitString[2*i] = c
	}
	// and all odd values from the y coordinate
	for i, c := range yBitString {
		xyBitString[2*i+1] = c
	}

	return string(xyBitString), zBitString
}

// https://lemire.me/blog/2018/01/08/how-fast-can-you-bit-interleave-32-bit-integers/
// (GitHub says the code is public domain)
func interleaveUint32WithZeros(input uint32) uint64 {
	var word uint64 = uint64(input)

	word = (word ^ (word << 16)) & 0x0000ffff0000ffff
	word = (word ^ (word << 8)) & 0x00ff00ff00ff00ff
	word = (word ^ (word << 4)) & 0x0f0f0f0f0f0f0f0f
	word = (word ^ (word << 2)) & 0x3333333333333333
	word = (word ^ (word << 1)) & 0x5555555555555555

	return word
}

func (pair BitStringPair) RawBitStringPair() RawBitStringPair {
	// https://lemire.me/blog/2018/01/08/how-fast-can-you-bit-interleave-32-bit-integers/
	XYBitString := interleaveUint32WithZeros(pair.xMin) | (interleaveUint32WithZeros(pair.yMin) << 1)

	return RawBitStringPair{
		XYBitString:    XYBitString,
		XYBitStringLen: pair.xPrecision + pair.yPrecision,
		ZBitString:     pair.zMin,
		ZBitStringLen:  pair.zPrecision,
	}
}

// Returns the geodetic coordinate corresponding to the point with the
// smallest longitude, latitude and altitude.
func (pair BitStringPair) GeodeticCoordinates() (float64, float64, float64) {
	var longitude float64 = ((float64(pair.xMin)*360)/float64(C_X+1) - 180)
	var latitude float64 = ((float64(pair.yMin)*180)/float64(C_Y+1) - 90)
	var altitude float64 = float64(int32(D) + int32(pair.zMin))

	return longitude, latitude, altitude
}

// Grows (*modifies*) the voxel by decreasing the `x` and `y` precision 'steps' times.
// If the precision of `x` and `y` is equal, the `y` precision is reduced,
// otherwise the `x` precision. Multiplies the covered area by `2 ** steps`
// Throws an exception if it is not possible to grow `steps` times
func (pair BitStringPair) Grow2D() {

}

// Grows (*modifies*) the voxel by decreasing the `z` precision `steps` times.
// Throws an exception if the it is not possible to grow `z` times.
// Multiplies the covered altitude by `2 ** steps.`
func (pair BitStringPair) GrowZ() {

}

// Grows (*modifies*) the voxel by removing bits from the 2d bit string until the voxel's
// shadow projected to the earth's surface (`.to_shapely_area()`) would be greater than
// `max_area` if another bit was removed.
// The area units are not meaningful as it is the result of computing an area using geodetic
// coordinates and treating them as certesian coordinates. Still this function can be useful
// for approximating an area.
func (pair BitStringPair) Grow2DToCoverArea() {

}

// Grows (*modifies*) the voxel by removing bits from the z bit string until the voxel's
// altitude would be greater than `max_altitude_range` if another bit was removed.
func (pair BitStringPair) GrowZToLength(altitudeMaxRange float64) {

}

func (pair BitStringPair) A(altitudeMaxRange float64) {

}
