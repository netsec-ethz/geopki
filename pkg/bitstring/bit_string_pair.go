package bitstring

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/golang/geo/r1"
	"github.com/golang/geo/s1"
	"github.com/golang/geo/s2"
	geo "github.com/kellydunn/golang-geo"
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

var DELTAS = []int32{-1, 0, 1}

type XYBitString struct {
	// The smallest x, and y coordinates of the voxel.
	// Must be in `[0, C_X]` and `[0, C_Y]` respectively.
	xMin, yMin uint32

	// The number of bits used to encode `xMin` and `yMin`, respectively.
	// Must be in `[0, X_BITS]` and `[0, Y_BITS]`, respectively.
	xPrecision, yPrecision uint8
}

type ZBitString struct {
	// The smallest z coordinate of the voxel.
	// Must be in `[0, C_Z]``
	zMin uint16

	// The number of bits used to encode `zMin`
	// Must be in `[0, Z_BITS]`
	zPrecision uint8
}

// this struct is analogous to the 'DiscretizedVoxel' class in '../coordinatez.py'
type BitStringPair struct {
	XYBitString
	ZBitString
}

// Creates a new bit string pair instance and ensures the passed values represent
// a valid bit string pair.
func NewBitStringPair(
	xMin, yMin uint32,
	zMin uint16,
	xPrecision, yPrecision, zPrecision uint8,
) (*BitStringPair, error) {

	// ensure xMin, yMin and zMin are in their respective ranges
	if xMin > C_X {
		return nil, fmt.Errorf(
			"xMin (%d) is greater than C_X (%d)",
			xMin,
			C_X,
		)
	}

	if yMin > C_Y {
		return nil, fmt.Errorf(
			"yMin (%d) is greater than C_Y (%d)",
			yMin,
			C_Y,
		)
	}

	if zMin > C_Z {
		return nil, fmt.Errorf(
			"zMin (%d) is greater than C_Z (%d)",
			zMin,
			C_Z,
		)
	}

	// ensure xPrecision, yPrecision and zPrecision are in their respective ranges
	if xPrecision > X_BITS {
		return nil, fmt.Errorf(
			"xPrecision (%d) is greater than X_BITS (%d)",
			xPrecision,
			X_BITS,
		)
	}

	if yPrecision > Y_BITS {
		return nil, fmt.Errorf(
			"yPrecision (%d) is greater than Y_BITS (%d)",
			yPrecision,
			Y_BITS,
		)
	}

	if zPrecision > Z_BITS {
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

	var xMask uint32 = uint32(math.MaxUint32) << (X_BITS - xPrecision)
	var yMask uint32 = uint32(math.MaxUint32) << (Y_BITS - yPrecision)
	var zMask uint16 = uint16(math.MaxUint16) << (Z_BITS - zPrecision)

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
		XYBitString: XYBitString{
			xMin: xMin,
			yMin: yMin,

			xPrecision: xPrecision,
			yPrecision: yPrecision,
		},
		ZBitString: ZBitString{
			zMin:       zMin,
			zPrecision: zPrecision,
		},
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
	var xBitString = []rune(strings.Repeat("0", int(X_BITS)))
	var yBitString = []rune(strings.Repeat("0", int(Y_BITS)))

	for i, c := range xyBitString {
		if i%2 == 0 {
			xBitString[i/2] = c
		} else {
			yBitString[i/2] = c
		}
	}

	// interpret the bit strings as integers
	x, err := strconv.ParseUint(string(xBitString), 2, int(X_BITS))
	if err != nil {
		return nil, err
	}

	y, err := strconv.ParseUint(string(yBitString), 2, int(Y_BITS))
	if err != nil {
		return nil, err
	}

	z, err := strconv.ParseUint(zBitString+strings.Repeat("0", int(Z_BITS)-len(zBitString)), 2, int(Z_BITS))
	if err != nil {
		return nil, err
	}

	pair := &BitStringPair{
		XYBitString: XYBitString{
			xMin: uint32(x),
			yMin: uint32(y),

			xPrecision: xPrecision,
			yPrecision: yPrecision,
		},
		ZBitString: ZBitString{
			zMin:       uint16(z),
			zPrecision: uint8(len(zBitString)),
		},
	}

	return pair, nil
}

// Creates a BitStringPair instance from a geodetic coordinate
func XYBitStringFromGeodeticCoordinates(longitude, latitude float64) (*XYBitString, error) {

	// ensure the passed longitude and latitude values are valid
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

	var x uint32 = uint32(((longitude + 180) / 360) * float64(C_X))
	var y uint32 = uint32(((latitude + 90) / 180) * float64(C_Y))

	// create an instance with the full precision
	return &XYBitString{
		xMin: x,
		yMin: y,

		xPrecision: X_BITS,
		yPrecision: Y_BITS,
	}, nil
}

// Creates a BitStringPair instance from a geodetic coordinate
func ZBitStringFromGeodeticCoordinate(altitude float64) (*ZBitString, error) {

	if altitude < float64(D) || altitude > float64(H) {
		return nil, fmt.Errorf(
			"altitudes must be in the range [%d, %d], %f given",
			D,
			H,
			altitude,
		)
	}

	var z uint16 = uint16(altitude - float64(D))

	// create an instance with the full precision
	return &ZBitString{
		zMin:       z,
		zPrecision: Z_BITS,
	}, nil
}

// Creates a BitStringPair instance from a geodetic coordinate
func BitStringPairFromGeodeticCoordinates(
	longitude, latitude, altitude float64,
) (*BitStringPair, error) {

	xyBitString, err := XYBitStringFromGeodeticCoordinates(longitude, latitude)
	if err != nil {
		return nil, err
	}

	zBitString, err := ZBitStringFromGeodeticCoordinate(altitude)
	if err != nil {
		return nil, err
	}

	// create an instance with the full precision
	pair := &BitStringPair{
		XYBitString: *xyBitString,
		ZBitString:  *zBitString,
	}

	return pair, nil
}

// The bit string encoding the `xMin` value, i.e. the `xPrecision` MSBs
func (bitString *XYBitString) XBitString() string {
	return fmt.Sprintf(
		// left-pad with 0s to X_BITS
		"%0*s",
		X_BITS,
		// convert integer to bit string
		strconv.FormatInt(int64(bitString.xMin), 2),
		// only use the first xPrecision (most significant) bits
	)[:bitString.xPrecision]
}

// The bit string encoding the `yMin` value, i.e. the `yPrecision` MSBs
func (bitString *XYBitString) YBitString() string {
	return fmt.Sprintf(
		// left-pad with 0s to Y_BITS
		"%0*s",
		Y_BITS,
		// convert integer to bit string
		strconv.FormatInt(int64(bitString.yMin), 2),
		// only use the first yPrecision (most significant) bits
	)[:bitString.yPrecision]
}

// The bit string encoding the `zMin` value, i.e. the `zPrecision` MSBs
func (bitSring *ZBitString) BitString() string {
	return fmt.Sprintf(
		// left-pad with 0s to Z_BITS
		"%0*s",
		Z_BITS,
		// convert integer to bit string
		strconv.FormatInt(int64(bitSring.zMin), 2),
		// only use the first zPrecision (most significant) bits
	)[:bitSring.zPrecision]
}

// The smallest discretized `x` coordinate that is no longer in the voxel
func (bitString *XYBitString) XMax() uint32 {
	// conceptually set the X_BITS - xPrecision least significant bits to 1
	// and add one to the resulting integer

	return bitString.xMin + (1 << (X_BITS - bitString.xPrecision))
}

// The smallest discretized `y` coordinate that is no longer in the voxel
func (bitString *XYBitString) YMax() uint32 {
	// conceptually set the Y_BITS - yPrecision least significant bits to 1
	// and add one to the resulting integer

	return bitString.yMin + (1 << (Y_BITS - bitString.yPrecision))
}

// The smallest discretized `z` coordinate that is no longer in the voxel
func (bitString *ZBitString) ZMax() uint16 {
	// conceptually set the Z_BITS - zPrecision least significant bits to 1
	// and add one to the resulting integer

	return bitString.zMin + (1 << (Z_BITS - bitString.zPrecision))
}

// Returns a string representation of the discretized coordinates
func (pair *BitStringPair) String() string {
	return "" +
		fmt.Sprintf("X: [%d, %d)", pair.xMin, pair.XMax()) + "\n" +
		fmt.Sprintf("Y: [%d, %d)", pair.yMin, pair.YMax()) + "\n" +
		fmt.Sprintf("Z: [%d, %d)", pair.zMin, pair.ZMax())
}

// Returns the bit string representation of the `x` and `y` coordinate and the
// bit representation of the `z` coordinate.
func (pair *BitStringPair) BitStringPair() (string, string) {
	xBitString := pair.XBitString()
	yBitString := pair.YBitString()
	zBitString := pair.ZBitString.BitString()

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
// (REAME on Github says the code is public domain)
func interleaveUint32WithZeros(input uint32) uint64 {
	var word uint64 = uint64(input)

	word = (word ^ (word << 16)) & 0x0000ffff0000ffff
	word = (word ^ (word << 8)) & 0x00ff00ff00ff00ff
	word = (word ^ (word << 4)) & 0x0f0f0f0f0f0f0f0f
	word = (word ^ (word << 2)) & 0x3333333333333333
	word = (word ^ (word << 1)) & 0x5555555555555555

	return word
}

func (bitString *XYBitString) RawXYBitStringPair() RawXYBitString {
	// https://lemire.me/blog/2018/01/08/how-fast-can-you-bit-interleave-32-bit-integers/
	XYBitString := interleaveUint32WithZeros(bitString.xMin) | (interleaveUint32WithZeros(bitString.yMin) << 1)

	return RawXYBitString{
		XYBitString:    XYBitString,
		XYBitStringLen: bitString.xPrecision + bitString.yPrecision,
	}
}

func (bitString *ZBitString) RawZBitStringPair() RawZBitString {
	return RawZBitString{
		ZBitString:    bitString.zMin,
		ZBitStringLen: bitString.zPrecision,
	}
}

func (pair *BitStringPair) RawBitStringPair() RawBitStringPair {
	return RawBitStringPair{
		RawXYBitString: pair.RawXYBitStringPair(),
		RawZBitString:  pair.RawZBitStringPair(),
	}
}

func UndiscretizeX(x uint32) float64 {
	return (float64(uint64(x)*360)/float64(C_X+1) - 180)
}

func UndiscretizeY(y uint32) float64 {
	return (float64(uint64(y)*180)/float64(C_Y+1) - 90)
}

func UndiscretizeZ(z uint16) float64 {
	return float64(int32(D) + int32(z))
}

// Returns the geodetic coordinate corresponding to the point with the
// smallest longitude, latitude and altitude.
func Undiscretize(x, y uint32, z uint16) (float64, float64, float64) {
	var longitude float64 = UndiscretizeX(x)
	var latitude float64 = UndiscretizeY(y)
	var altitude float64 = UndiscretizeZ(z)

	return longitude, latitude, altitude
}

// Returns the geodetic coordinate corresponding to the point with the
// smallest longitude, latitude
func (bitString *XYBitString) GeodeticCoordinates() (float64, float64) {
	return UndiscretizeX(bitString.xMin), UndiscretizeY(bitString.yMin)
}

// Returns the geodetic coordinate corresponding to the point with the
// smallest altitude
func (bitString *ZBitString) GeodeticCoordinates() float64 {
	return UndiscretizeZ(bitString.zMin)
}

// Returns the geodetic coordinate corresponding to the point with the
// smallest longitude, latitude and altitude.
func (pair *BitStringPair) GeodeticCoordinates() (float64, float64, float64) {
	return UndiscretizeX(pair.xMin), UndiscretizeY(pair.yMin), UndiscretizeZ(pair.zMin)
}

// Returns a two dimensional boundary of the voxel's projection
// to the earth's surface in geodetic coordinates.
func (bitString *XYBitString) Rect() s2.Rect {
	longitude_min := UndiscretizeX(bitString.xMin)
	longitude_max := UndiscretizeX(bitString.XMax())

	latitude_min := UndiscretizeY(bitString.yMin)
	latitude_max := UndiscretizeY(bitString.YMax())

	return s2.Rect{
		Lng: s1.Interval{Lo: longitude_min, Hi: longitude_max},
		Lat: r1.Interval{Lo: latitude_min, Hi: latitude_max},
	}
}

// Returns a two dimensional boundary of the voxel's projection
// to the earth's surface in geodetic coordinates.
func (bitString *XYBitString) Loop() *s2.Loop {
	longitude_min := UndiscretizeX(bitString.xMin)
	longitude_max := UndiscretizeX(bitString.XMax())

	latitude_min := UndiscretizeY(bitString.yMin)
	latitude_max := UndiscretizeY(bitString.YMax())

	// counter-clockwise orientation for non-holes
	// https://pkg.go.dev/github.com/golang/geo/s2#example-PolygonFromOrientedLoops
	points := [][]float64{
		{longitude_min, latitude_min},
		{longitude_max, latitude_min},
		{longitude_max, latitude_max},
		{longitude_min, latitude_max},
	}

	var pts []s2.Point
	for _, pt := range points {
		pts = append(pts, s2.PointFromLatLng(s2.LatLngFromDegrees(pt[1], pt[0])))
	}
	return s2.LoopFromPoints(pts)
}

// Grows (*modifies*) the voxel by decreasing the `x` and `y` precision 'steps' times.
// If the precision of `x` and `y` is equal, the `y` precision is reduced,
// otherwise the `x` precision. Multiplies the covered area by `2 ** steps`
// Throws an exception if it is not possible to grow `steps` times
func (bitString *XYBitString) Grow2D(steps uint8) error {
	// Invariant `(x_precision == y_precision) or (x_precision == y_precision + 1)` must hold

	var xBitsToClear uint8
	var yBitsToClear uint8

	if bitString.xPrecision+bitString.yPrecision <= steps {
		// cannot grow further, one bit must be left in the end
		return fmt.Errorf("cannot grow further in 2D, only one bit left")
	} else if bitString.xPrecision == bitString.yPrecision {
		// start with the y bit, if odd clear one more y bit
		xBitsToClear = steps / 2
		yBitsToClear = (steps + 1) / 2
	} else if bitString.xPrecision == bitString.yPrecision+1 {
		// start with the x bit, if odd clear one more x bit
		xBitsToClear = (steps + 1) / 2
		yBitsToClear = steps / 2
	} else {
		return fmt.Errorf("(xPrecision == yPrecision) or (xPrecision == yPrecision + 1) invariant violated")
	}

	// clear all bits starting from xPrecision to xPrecision + xBitsToClear
	xBitMask := uint32(math.MaxUint32) << (X_BITS - (bitString.xPrecision - xBitsToClear))
	bitString.xMin &= xBitMask
	bitString.xPrecision -= xBitsToClear

	// clear all bits starting from yPrecision to yPrecision + yBitsToClear
	yBitMask := uint32(math.MaxUint32) << (Y_BITS - (bitString.yPrecision - yBitsToClear))
	bitString.yMin &= yBitMask
	bitString.yPrecision -= yBitsToClear

	return nil
}

// Grows (*modifies*) the voxel by decreasing the `z` precision `steps` times.
// Throws an exception if the it is not possible to grow `z` times.
// Multiplies the covered altitude by `2 ** steps.`
func (bitString *ZBitString) GrowZ(steps uint8) error {
	if bitString.zPrecision <= steps {
		// cannot grow further
		return fmt.Errorf("cannot grow further in the altitude, only one bit left")
	}

	// clear all bits starting from yPrecision to yPrecision + yBitsToClear
	zBitMask := uint16(math.MaxUint16) << (Z_BITS - bitString.zPrecision - steps)
	bitString.zMin &= zBitMask
	bitString.zPrecision -= steps

	return nil
}

// Grows (*modifies*) the voxel by removing bits from the 2d bit string until the voxel's
// shadow projected to the earth's surface (`.Rect()`) would be greater than
// `max_area` if another bit was removed.
// The area units are the ones computed by the S2 library
func (bitString *XYBitString) Grow2DToCoverArea(maxArea float64) error {
	currentArea := bitString.Loop().Area()
	growSteps := math.Log2(maxArea / currentArea)

	if growSteps < 0 {
		// no shrinking
		return nil
	} else if growSteps > float64(bitString.xPrecision+bitString.yPrecision) {
		return fmt.Errorf("something seems off, cannot grow larger than the whole world")
	}

	return bitString.Grow2D(uint8(growSteps))
}

// Grows (*modifies*) the voxel by removing bits from the z bit string until the voxel's
// altitude would be greater than `altitudeMaxRange` if another bit was removed.
func (bitString *ZBitString) GrowZToLength(altitudeMaxRange float64) error {
	currentAltitudeRange := float64(bitString.ZMax() - bitString.zMin + 1)
	growSteps := math.Log2(altitudeMaxRange / currentAltitudeRange)

	if growSteps < 0 {
		// no shrinking
		return nil
	} else if growSteps > float64(Z_BITS) {
		return fmt.Errorf("something seems off, cannot grow larger than the whole world")
	}

	return bitString.GrowZ(uint8(growSteps))
}

// Computes a set of 2D bit strings from a given set of polygons.
// `f_grow` and `f_min` are parameters influencing the accuracy
// of the approximation.
//
// The algorithm first computes the smallest voxel corresponding
// to a random polygon vertex. It then grows this voxel's until
// it's 2D shadow covers `f_grow` of the polygon's area.
//
// In a next step, a BFS among the voxel's neighbors is performed
// and the neighboring voxels are checked for intersection with
// the polygon.
//
// After the BFS, neighboring voxels intersecting the polygon are
// merged and only their parent bit string is returned.
// Redundant bit strings are omitted (e.g. ones where the result
// also contains a prefix of them).
//
// The level of the approximation's accuracy is determined by `f_grow`.
// By setting `f_grow = 0`, the best possible approximation is computed,
// resulting in more bit strings.
func PolygonsTo2DBitStrings(polygons []*s2.Loop, fGrow float64) ([]RawXYBitString, error) {
	intersectingAreasAllPolygons := mapset.NewSet[RawXYBitString]()

	for _, polygon := range polygons {
		// this will be the list of bitstrings of the chosen size for 'polygon'
		intersectingAreas := mapset.NewSet[RawXYBitString]()

		// perform the BFS
		visited := mapset.NewSet[RawXYBitString]()
		q := make([]*XYBitString, 0, 1)

		initialCoordinate := s2.LatLngFromPoint(polygon.Vertex(0))

		initialBitString, err := XYBitStringFromGeodeticCoordinates(
			initialCoordinate.Lng.Degrees(),
			initialCoordinate.Lat.Degrees(),
		)

		if err != nil {
			return nil, err
		}

		err = initialBitString.Grow2DToCoverArea(
			polygon.Area() * fGrow,
		)

		if err != nil {
			return nil, err
		}

		q = append(q, initialBitString)

		for len(q) > 0 {
			voxel := q[0] // top
			q = q[1:]     // pop queue

			xyBitStringPair := voxel.RawXYBitStringPair()

			if visited.Contains(xyBitStringPair) {
				continue
			}

			// mark as visited
			visited.Add(xyBitStringPair)

			// check for intersection. always take the first area
			// println(LoopToGeoJson(voxel.Loop()))
			if intersectingAreas.Cardinality() > 0 && !(voxel.Loop().Intersects(polygon)) {
				continue
			}

			// println(LoopToGeoJson(voxel.Loop()))

			// add to intersection list
			intersectingAreas.Add(xyBitStringPair)

			// visit neighbors of a
			for _, dx := range DELTAS {
				for _, dy := range DELTAS {

					// compute neighbor coordinates
					xNext := uint32((int32(voxel.xMin) + dx*(1<<(X_BITS-voxel.xPrecision))) % int32(C_X))

					yStep := int32(voxel.yMin) + dy*(1<<(Y_BITS-voxel.yPrecision))
					yNext := uint32(yStep)

					if yStep < 0 {
						// the y-coordinate 'flips', we can account for this
						// by only rotating around x and set y to 0 (smallest coordinate of voxel)
						yNext = 0
						// if we overflow, the x coordinate wraps around
						xNext = (xNext + (C_X / 2)) % C_X
					} else if yStep >= int32(C_Y) {
						// the y-coordinate 'flips', we can account for this
						// by rotating around x and set y to C_Y - step size = original y
						yNext = voxel.yMin
						// if we overflow the x coordinate wraps around
						xNext = (xNext + (C_X / 2)) % C_X
					}

					// clear bottom bits of the x coordinate, might be messed up after wrapping around
					xNext = xNext & (uint32(math.MaxUint32) << (X_BITS - voxel.xPrecision))

					q = append(
						q,
						&XYBitString{
							xMin: xNext,
							yMin: yNext,

							xPrecision: voxel.xPrecision,
							yPrecision: voxel.yPrecision,
						},
					)
				}
			}
		}

		// append `intersecting_areas` to list for all polygons
		intersectingAreasAllPolygons = intersectingAreasAllPolygons.Union(
			intersectingAreas,
		)

	}

	// after computing the intersecting voxels, merge them and remove redundant ones
	results := make([]RawXYBitString, 0, intersectingAreasAllPolygons.Cardinality())

	// transform set to list
	intersectingAreasAllPolygonsList := intersectingAreasAllPolygons.ToSlice()

	for bit_string_idx := 0; bit_string_idx < len(intersectingAreasAllPolygonsList); bit_string_idx++ {
		bitString := intersectingAreasAllPolygonsList[bit_string_idx]

		// check if this bit string is redundant, i.e. a shorter prefix is also
		// part of the set
		skip := false
		// iterate over all prefixes of that bitstring from largest/shortest to smallest/longest
		for i := uint8(0); i <= bitString.XYBitStringLen; i++ {
			// check if any of its prefixes (larger areas) is also part of intersectingAreasAllPolygonsList
			if intersectingAreasAllPolygons.Contains(bitString.Ancestor(i)) {
				// if it is, ignore this one as the certificate will be included in the larger/shorter
				// prefix
				skip = true
				break
			}
		}

		if skip {
			// ignore by skipping over this index
			continue
		}

		// check if area can be merged with neighbor
		if intersectingAreasAllPolygons.Contains(bitString.Neighbor()) {
			// yes it can. ignore current bit_string by skipping (continue)
			// if the neighbor is visited afterwards it will be skipped because
			// the list contains a prefix of it

			// add parent at the end of the list to make sure duplicate test is performed with parent again
			intersectingAreasAllPolygonsList = append(intersectingAreasAllPolygonsList, bitString.Parent())
		}

		// from this point on bit_string is sucessfully taken
		results = append(results, bitString)

	}

	return results, nil
}

// Approximates a circle defined by geodetic coordinates and a radius
// in meters using a shapely polygon in the eucledian geodetic space.
//
// First `radius_m` are walked in a few directions (bearing) from
// the center, then the eucledian distances to these points
// using in the eucledian geodetic space are computed and the
// maximum is used to approximate the circle.
//
// Using this radius, the circle is then approximated as a
// `4 * quadSegs` sided polygon.
func ApproximateSphere(longitude, latitude float64, radiusM uint8, quadSegs uint8) *s2.Loop {
	center := geo.NewPoint(latitude, longitude)
	radiusKm := float64(radiusM) / 1000

	// approximate circle, accuracy is slightly less important for correctness
	// as this is computed by the client

	segments := int(quadSegs) * 4
	segmentDegrees := 360.0 / float64(segments)

	pts := make([]s2.Point, segments)

	for i := 0; i < segments; i++ {

		// walk `radius_m` in a few directions (bearing) from the center
		position := center.PointAtDistanceAndBearing(
			radiusKm,
			float64(i)*segmentDegrees,
		)

		// ccw order
		pts[segments-1-i] = s2.PointFromLatLng(s2.LatLngFromDegrees(position.Lat(), position.Lng()))
	}

	return s2.LoopFromPoints(pts)
}

// Returns the single longest / most precise bit string encompassing both,
// `altitudeMin` and `altitudeMax`. In contrast to
// `PolygonsTo2DBitStrings`. Since it only returns
// a single bit string it is much more likely to use a shorter / less
// precise bit string than `polygons_to_2d_bit_strings` but
// results in a sparser tree. Under the assumption that the altitude
// is rather sparse this seems to be a good tradeoff.
func SmallestEnclosingZBitString(altitudeMin, altitudeMax float64) (*ZBitString, error) {
	bitString, err := ZBitStringFromGeodeticCoordinate(altitudeMin)
	if err != nil {
		return nil, err
	}

	altitudeMaxRange := altitudeMax - altitudeMin + 1
	currentAltitudeRange := float64(bitString.ZMax() - bitString.zMin + 1)
	growSteps := math.Ceil(math.Log2(altitudeMaxRange / currentAltitudeRange))

	if growSteps < 0 {
		// no shrinking
		return bitString, nil
	} else if growSteps > float64(Z_BITS) {
		return nil, fmt.Errorf("something seems off, cannot grow larger than the whole world")
	}

	err = bitString.GrowZ(uint8(growSteps))

	return bitString, err
}

// Returns the cross product of `PolygonsTo2DBitStrings` and
// `SmallestEnclosingZBitString` for the given parameters
// resulting in the set of all bit string pairs where a given
// extruded polygon should be assigned. Always over-approximates,
// i.e. covers the whole extruded polygon.
func ExtrudedPolygonsToBitStringPairs(
	polygons []*s2.Loop,
	altitudeMin, altitudeMax float64,
	fGrow float64,
) ([]RawBitStringPair, error) {

	xyBitStrings, err := PolygonsTo2DBitStrings(
		polygons,
		fGrow,
	)
	if err != nil {
		return nil, err
	}

	zBitString, err := SmallestEnclosingZBitString(
		altitudeMin,
		altitudeMax,
	)
	if err != nil {
		return nil, err
	}

	bitStringPairs := make([]RawBitStringPair, len(xyBitStrings))
	for i, xyBitString := range xyBitStrings {
		bitStringPairs[i] = RawBitStringPair{
			RawXYBitString: xyBitString,
			RawZBitString:  zBitString.RawZBitStringPair(),
		}
	}

	return bitStringPairs, nil
}

func LoopToGeoJson(loop *s2.Loop) string {
	vertices := (loop.Vertices())
	coordinates := make([]string, len(vertices)+1)
	for i, vertex := range vertices {
		coordinate := s2.LatLngFromPoint(vertex)
		coordinates[i] = fmt.Sprintf("[%f, %f]", coordinate.Lng.Degrees(), coordinate.Lat.Degrees())
	}
	coordinates[len(vertices)] = coordinates[0]

	return fmt.Sprintf(
		"{\"type\":\"Feature\",\"properties\":{},\"geometry\":{\"type\":\"Polygon\",\"coordinates\":[[%s]]}}",
		strings.Join(coordinates, ","),
	)
}
