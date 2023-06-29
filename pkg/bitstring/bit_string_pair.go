package bitstring

import (
	"fmt"
	"math"
	"math/bits"
	"strconv"
	"strings"

	mapset "github.com/deckarep/golang-set/v2"
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
	XMin, YMin uint32

	// The number of bits used to encode `xMin` and `yMin`, respectively.
	// Must be in `[0, X_BITS]` and `[0, Y_BITS]`, respectively.
	XPrecision, YPrecision uint8
}

type ZBitString struct {
	// The smallest z coordinate of the voxel.
	// Must be in `[0, C_Z]``
	ZMin uint16

	// The number of bits used to encode `zMin`
	// Must be in `[0, Z_BITS]`
	ZPrecision uint8
}

// this struct is analogous to the 'DiscretizedVoxel' class in '../coordinatez.py'
type BitStringPair struct {
	XYBitString
	ZBitString
}

// generic interface for computing intersections
// allows the dual use of gdal and s2 for the web demo
type Geometry2D interface {
	Intersects(bitstring *XYBitString) bool
	InitialXYBitString(fGrow float64) (*XYBitString, error)
	// some implementations might have to be freeded manually
	Destroy()
}

// the s2 implementation of the 'Geometry2D' interface
type S2Geometry2D struct {
	Loop *s2.Loop
}

func (g *S2Geometry2D) Intersects(xyBitstring *XYBitString) bool {
	return g.Loop.Intersects(xyBitstring.Loop())
}

func (g *S2Geometry2D) InitialXYBitString(fGrow float64) (*XYBitString, error) {
	polygon := g.Loop

	initialCoordinate := s2.LatLngFromPoint(polygon.Vertex(0))

	initialBitString, err := XYBitStringFromGeodeticCoordinates(
		initialCoordinate.Lng.Degrees(),
		initialCoordinate.Lat.Degrees(),
	)
	if err != nil {
		return nil, err
	}

	// grow initial bitstring to 'maxArea'
	maxArea := polygon.Area() * fGrow
	currentArea := initialBitString.Loop().Area()

	growSteps := math.Log2(maxArea / currentArea)

	if growSteps < 0 {
		// no shrinking is done so do nothing
	} else if growSteps > float64(initialBitString.XPrecision+initialBitString.YPrecision) {
		return nil, fmt.Errorf("something seems off, cannot grow larger than the whole world (%f / %f = %f > %d;)", maxArea, currentArea, growSteps, initialBitString.XPrecision+initialBitString.YPrecision)
	}

	err = initialBitString.Grow2D(uint8(growSteps))
	if err != nil {
		return nil, err
	}

	return initialBitString, nil
}

func (g *S2Geometry2D) Destroy() {
	// just a go object, no need to manually free
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
			XMin: xMin,
			YMin: yMin,

			XPrecision: xPrecision,
			YPrecision: yPrecision,
		},
		ZBitString: ZBitString{
			ZMin:       zMin,
			ZPrecision: zPrecision,
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
			XMin: uint32(x),
			YMin: uint32(y),

			XPrecision: xPrecision,
			YPrecision: yPrecision,
		},
		ZBitString: ZBitString{
			ZMin:       uint16(z),
			ZPrecision: uint8(len(zBitString)),
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
		// 0x3ffffff = uint32(math.MaxUint32) >> (32 - X_BITS), go complains if written out..
		XMin: (x) & 0x3ffffff,
		// 0x1ffffff = (uint32(math.MaxUint32) >> (32 - Y_BITS)), go complains if written out..
		YMin: (y) & 0x1ffffff,

		XPrecision: X_BITS,
		YPrecision: Y_BITS,
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
		ZMin:       z,
		ZPrecision: Z_BITS,
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
		strconv.FormatInt(int64(bitString.XMin), 2),
		// only use the first xPrecision (most significant) bits
	)[:bitString.XPrecision]
}

// The bit string encoding the `yMin` value, i.e. the `yPrecision` MSBs
func (bitString *XYBitString) YBitString() string {
	return fmt.Sprintf(
		// left-pad with 0s to Y_BITS
		"%0*s",
		Y_BITS,
		// convert integer to bit string
		strconv.FormatInt(int64(bitString.YMin), 2),
		// only use the first yPrecision (most significant) bits
	)[:bitString.YPrecision]
}

// The bit string encoding the `zMin` value, i.e. the `zPrecision` MSBs
func (bitString *ZBitString) String() string {
	return fmt.Sprintf(
		// left-pad with 0s to Z_BITS
		"%0*s",
		Z_BITS,
		// convert integer to bit string
		strconv.FormatInt(int64(bitString.ZMin), 2),
		// only use the first zPrecision (most significant) bits
	)[:bitString.ZPrecision]
}

// The smallest discretized `x` coordinate that is no longer in the voxel
func (bitString *XYBitString) XMax() uint32 {
	// conceptually set the X_BITS - xPrecision least significant bits to 1
	// and add one to the resulting integer

	return bitString.XMin + (1 << (X_BITS - bitString.XPrecision))
}

// The smallest discretized `y` coordinate that is no longer in the voxel
func (bitString *XYBitString) YMax() uint32 {
	// conceptually set the Y_BITS - yPrecision least significant bits to 1
	// and add one to the resulting integer

	return bitString.YMin + (1 << (Y_BITS - bitString.YPrecision))
}

// The smallest discretized `z` coordinate that is no longer in the voxel
func (bitString *ZBitString) ZMax() uint16 {
	// conceptually set the Z_BITS - zPrecision least significant bits to 1
	// and add one to the resulting integer

	return bitString.ZMin + (1 << (Z_BITS - bitString.ZPrecision))
}

// Returns a string representation of the discretized coordinates
func (pair *BitStringPair) String() string {
	return "" +
		fmt.Sprintf("X: [%d, %d)", pair.XMin, pair.XMax()) + "\n" +
		fmt.Sprintf("Y: [%d, %d)", pair.YMin, pair.YMax()) + "\n" +
		fmt.Sprintf("Z: [%d, %d)", pair.ZMin, pair.ZMax())
}

func (b *XYBitString) String() string {
	xBitString := b.XBitString()
	yBitString := b.YBitString()

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

	return string(xyBitString)
}

// Returns the bit string representation of the `x` and `y` coordinate and the
// bit representation of the `z` coordinate.
func (pair *BitStringPair) BitStringPair() (string, string) {
	return pair.XYBitString.String(), pair.ZBitString.String()
}

// https://lemire.me/blog/2018/01/08/how-fast-can-you-bit-interleave-32-bit-integers/
// (REAME on Github says the code is public domain)
// starts with zero
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
	// first shift numbers to move the used bits from the end to the start
	xBitString := bitString.XMin << (32 - X_BITS)
	yBitString := bitString.YMin << (32 - Y_BITS)

	// https://lemire.me/blog/2018/01/08/how-fast-can-you-bit-interleave-32-bit-integers/
	xZeroInterleaved := (interleaveUint32WithZeros(xBitString) << 1)
	yZeroInterleaved := interleaveUint32WithZeros(yBitString)

	XYBitString := xZeroInterleaved | yZeroInterleaved

	return RawXYBitString{
		XYBitString:    XYBitString,
		XYBitStringLen: bitString.XPrecision + bitString.YPrecision,
	}
}

func (bitString *ZBitString) RawZBitString() RawZBitString {
	return RawZBitString{
		// move the used bits from the end to the start
		ZBitString:    bitString.ZMin << (16 - Z_BITS),
		ZBitStringLen: bitString.ZPrecision,
	}
}

func (pair *BitStringPair) RawBitStringPair() RawBitStringPair {
	return RawBitStringPair{
		RawXYBitString: pair.RawXYBitStringPair(),
		RawZBitString:  pair.RawZBitString(),
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
	return UndiscretizeX(bitString.XMin), UndiscretizeY(bitString.YMin)
}

// Returns the geodetic coordinate corresponding to the point with the
// smallest altitude
func (bitString *ZBitString) GeodeticCoordinate() float64 {
	return UndiscretizeZ(bitString.ZMin)
}

// Returns the geodetic coordinate corresponding to the point with the
// smallest longitude, latitude and altitude.
func (pair *BitStringPair) GeodeticCoordinates() (float64, float64, float64) {
	return UndiscretizeX(pair.XMin), UndiscretizeY(pair.YMin), UndiscretizeZ(pair.ZMin)
}

// Returns a two dimensional boundary of the voxel's projection
// to the earth's surface in geodetic coordinates.
func (bitString *XYBitString) Loop() *s2.Loop {
	longitude_min := UndiscretizeX(bitString.XMin)
	longitude_max := UndiscretizeX(bitString.XMax())

	latitude_min := UndiscretizeY(bitString.YMin)
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

	if bitString.XPrecision+bitString.YPrecision <= steps {
		// cannot grow further, one bit must be left in the end
		return fmt.Errorf("cannot grow further in 2D, %d bits left and tried growing by %d bits", bitString.XPrecision+bitString.YPrecision, steps)
	} else if bitString.XPrecision == bitString.YPrecision {
		// start with the y bit, if odd clear one more y bit
		xBitsToClear = steps / 2
		yBitsToClear = (steps + 1) / 2
	} else if bitString.XPrecision == bitString.YPrecision+1 {
		// start with the x bit, if odd clear one more x bit
		xBitsToClear = (steps + 1) / 2
		yBitsToClear = steps / 2
	} else {
		return fmt.Errorf("(xPrecision == yPrecision) or (xPrecision == yPrecision + 1) invariant violated")
	}

	// clear all bits outside the new precision, used bits are at the end
	bitString.XPrecision -= xBitsToClear
	bitString.XMin = bitString.XMin & (uint32(math.MaxUint32) << (X_BITS - bitString.XPrecision))

	// clear all bits outside the new precision
	bitString.YPrecision -= yBitsToClear
	bitString.YMin = bitString.YMin & (uint32(math.MaxUint32) << (Y_BITS - bitString.YPrecision))

	return nil
}

// Grows (*modifies*) the voxel by decreasing the `z` precision `steps` times.
// Throws an exception if the it is not possible to grow `z` times.
// Multiplies the covered altitude by `2 ** steps.`
func (bitString *ZBitString) GrowZ(steps uint8) error {
	if bitString.ZPrecision < steps {
		// cannot grow further
		return fmt.Errorf("cannot grow further in the altitude")
	}

	// clear all bits starting from after the new precision (bitString.ZPrecision - steps)
	bitString.ZPrecision -= steps
	bitString.ZMin = bitString.ZMin & (uint16(math.MaxUint16) << (Z_BITS - bitString.ZPrecision))

	return nil
}

// Grows (*modifies*) the voxel by removing bits from the z bit string until the voxel's
// altitude would be greater than `altitudeMaxRange` if another bit was removed.
func (bitString *ZBitString) GrowZToLength(altitudeMaxRange float64) error {
	currentAltitudeRange := float64(bitString.ZMax() - bitString.ZMin)
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
func PolygonsTo2DBitStrings(polygons []Geometry2D, fGrow float64) ([]RawXYBitString, error) {
	intersectingAreasAllPolygons := mapset.NewThreadUnsafeSet[RawXYBitString]()

	for _, polygon := range polygons {

		// this will be the list of bitstrings of the chosen size for 'polygon'
		intersectingAreas := mapset.NewThreadUnsafeSet[RawXYBitString]()

		// perform the BFS
		visited := mapset.NewThreadUnsafeSet[RawXYBitString]()
		q := make([]*XYBitString, 0, 1)

		initialBitString, err := polygon.InitialXYBitString(fGrow)

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

			// check for intersection
			if !(polygon.Intersects(voxel)) {
				continue
			}

			// println(LoopToGeoJson(voxel.Loop()))

			// add to intersection list
			intersectingAreas.Add(xyBitStringPair)

			// visit neighbors of a
			for _, dx := range DELTAS {
				for _, dy := range DELTAS {

					// compute neighbor coordinates
					xNext := uint32((int32(voxel.XMin) + dx*(1<<(X_BITS-voxel.XPrecision))) % int32(C_X))

					yStep := int32(voxel.YMin) + dy*(1<<(Y_BITS-voxel.YPrecision))
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
						yNext = voxel.YMin
						// if we overflow the x coordinate wraps around
						xNext = (xNext + (C_X / 2)) % C_X
					}

					// clear bottom bits of the x coordinate, might be messed up after wrapping around
					xNext = xNext & (uint32(math.MaxUint32) << (X_BITS - voxel.XPrecision))

					q = append(
						q,
						&XYBitString{
							XMin: xNext,
							YMin: yNext,

							XPrecision: voxel.XPrecision,
							YPrecision: voxel.YPrecision,
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
		for i := uint8(1); i <= bitString.XYBitStringLen; i++ {
			// check if any of its prefixes (larger areas) is also part of intersectingAreasAllPolygons
			// do not need to check updated list after merging neighbors because the covered "area"
			// does not change, it could only be a ancestor further up in the tree
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
			continue
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
func ApproximateCircle(longitude, latitude float64, radiusM uint8, quadSegs uint8) *s2.Loop {
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
// `PolygonsTo2DBitStrings` it only returns
// a single bit string which results in a shorter / less
// precise bit string than `polygons_to_2d_bit_strings` but
// makes the tree sparser. Under the assumption that the altitude
// is rather sparse this seems to be a good tradeoff.
func SmallestEnclosingZBitString(altitudeMin, altitudeMax float64) (*ZBitString, error) {
	bitStringMin, err := ZBitStringFromGeodeticCoordinate(altitudeMin)
	if err != nil {
		return nil, err
	}

	bitStringMax, err := ZBitStringFromGeodeticCoordinate(altitudeMax)
	if err != nil {
		return nil, err
	}

	rawBitStringMin := bitStringMin.RawZBitString()
	rawBitStringMax := bitStringMax.RawZBitString()

	matchingPrefixLength := bits.LeadingZeros16(rawBitStringMin.ZBitString ^ rawBitStringMax.ZBitString)
	if matchingPrefixLength > int(Z_BITS) {
		matchingPrefixLength = int(Z_BITS)
	}

	err = bitStringMin.GrowZ(bitStringMin.ZPrecision - uint8(matchingPrefixLength))

	return bitStringMin, err
}

// Returns the cross product of `PolygonsTo2DBitStrings` and
// `SmallestEnclosingZBitString` for the given parameters
// resulting in the set of all bit string pairs where a given
// extruded polygon should be assigned. Always over-approximates,
// i.e. covers the whole extruded polygon.
func ExtrudedPolygonsToBitStringPairs(
	polygons []Geometry2D,
	altitudeMin, altitudeMax float64,
	fGrow float64,
) ([]*RawBitStringPair, error) {

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

	bitStringPairs := make([]*RawBitStringPair, len(xyBitStrings))
	for i, xyBitString := range xyBitStrings {
		bitStringPairs[i] = &RawBitStringPair{
			RawXYBitString: xyBitString,
			RawZBitString:  zBitString.RawZBitString(),
		}
	}

	return bitStringPairs, nil
}

func LoopToGeoPolygon(loop *s2.Loop) string {
	vertices := (loop.Vertices())
	coordinates := make([]string, len(vertices)+1)
	for i, vertex := range vertices {
		coordinate := s2.LatLngFromPoint(vertex)
		coordinates[i] = fmt.Sprintf("[%f, %f]", coordinate.Lng.Degrees(), coordinate.Lat.Degrees())
	}
	coordinates[len(vertices)] = coordinates[0]

	return fmt.Sprintf(
		"{\"type\":\"Polygon\",\"coordinates\":[[%s]]}",
		strings.Join(coordinates, ","),
	)
}

func LoopToGeoJsonFeature(loop *s2.Loop) string {
	return fmt.Sprintf(
		"{\"type\":\"Feature\",\"properties\":{},\"geometry\":%s}",
		LoopToGeoPolygon(loop),
	)
}
