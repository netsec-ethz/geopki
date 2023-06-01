package bitstring

import (
	"fmt"
	"math"
)

type RawXYBitString struct {
	// the bit string encoding the x and y coordinate
	//[7]byte interpreted as a big endian integer
	XYBitString uint64
	// the number of most siginifanct / first bits that are used
	// has to be in [0, 52]
	XYBitStringLen uint8
}

type RawZBitString struct {
	// the bit string encoding the z coordinate
	//[2]byte interpreted as a big endian integer
	ZBitString uint16
	// the number of most siginifanct / first bits that are used
	// has to be in [0, 15]
	ZBitStringLen uint8
}

type RawBitStringPair struct {
	// the bit string encoding the x and y coordinate
	RawXYBitString

	// the bit string encoding the z coordinate
	RawZBitString
}

func min(a, b uint8) uint8 {
	if a < b {
		return a
	}
	return b
}

func deInterleaveOddBits(word uint64) uint32 {
	// 0xAA = 10101010, alternating ones and zeros; starting with one
	// 0x55 = 01010101, alternating ones and zeros; starting with zero

	// zero all even bits
	word = (word & 0x5555555555555555)
	// after this we only want to move the bits closer together
	// 0x33 = 00110011, alternating pairs of zeros and ones; starting with zeros
	// copies odd bits to (cleared) even position to the right, leaves odd bits
	// then masks out every other odd bit, they bits already moved closer together
	// in groups of two
	word = (word | (word >> 1)) & 0x3333333333333333
	// 0x0F = 00001111, similar process to before but now groups of four
	word = (word | (word >> 2)) & 0x0f0f0f0f0f0f0f0f
	// similar process to before but now groups of 8
	word = (word | (word >> 4)) & 0x00ff00ff00ff00ff
	// similar process to before but now groups of 16
	word = (word | (word >> 8)) & 0x0000ffff0000ffff
	// similar process to before but now groups of 32
	word = (word | (word >> 16)) & 0x00000000ffffffff

	return uint32(word)
}

// de-interleaves the bits, returns the a pair of (even, odd) bits
func deInterleaveUint64(input uint64) (uint32, uint32) {
	// shift input by one bit to the right (make even bits odd and vice versa)
	// last bit (51th) is an odd one, will become even but is then masked away
	return deInterleaveOddBits(input >> 1), deInterleaveOddBits(input)
}

// returns the bit string pair as strings
func (b RawXYBitString) BitString() *XYBitString {
	xMin, yMin := deInterleaveUint64(b.XYBitString)

	return &XYBitString{
		// move the relevant bits to the end
		xMin:       xMin >> (32 - X_BITS),
		xPrecision: (b.XYBitStringLen + 1) / 2,
		// move the relevant bits to the end
		yMin:       yMin >> (32 - Y_BITS),
		yPrecision: b.XYBitStringLen / 2,
	}
}

// returns the bit string pair as strings
func (pair RawBitStringPair) BitStringPair() *BitStringPair {
	return &BitStringPair{
		XYBitString: *pair.RawXYBitString.BitString(),
		ZBitString: ZBitString{
			zMin:       pair.ZBitString >> (16 - Z_BITS),
			zPrecision: pair.ZBitStringLen,
		},
	}
}

// checks if the node is the root node
func (pair RawBitStringPair) IsRoot() bool {
	return pair.XYBitStringLen == 0 && pair.ZBitStringLen == 0
}

// checks for equality
func (pair RawBitStringPair) Equals(other RawBitStringPair) bool {
	return pair.XYBitStringLen == other.XYBitStringLen && pair.ZBitStringLen == other.ZBitStringLen && pair.XYBitString == other.XYBitString && pair.ZBitString == other.ZBitString
}

// returns an ancestor
func (pair *RawXYBitString) Ancestor(levels uint8) RawXYBitString {
	if pair.XYBitStringLen <= levels {
		// return a root node instance
		return RawXYBitString{
			XYBitString:    0,
			XYBitStringLen: 0,
		}
	}

	return RawXYBitString{
		XYBitString:    pair.XYBitString & (math.MaxUint64 << (64 - (pair.XYBitStringLen - levels))),
		XYBitStringLen: pair.XYBitStringLen - levels,
	}
}

// returns an ancestor
func (pair *RawZBitString) Ancestor(levels uint8) RawZBitString {
	if pair.ZBitStringLen <= levels {
		// return a root node instance
		return RawZBitString{
			ZBitString:    0,
			ZBitStringLen: 0,
		}
	}

	return RawZBitString{
		ZBitString:    pair.ZBitString & (math.MaxUint16 << (16 - (pair.ZBitStringLen - levels))),
		ZBitStringLen: pair.ZBitStringLen - levels,
	}
}

// returns XYBitString and ZBitString of an ancestor as a pair struct
func (pair *RawBitStringPair) AncestorPair(levels uint8) RawBitStringPair {

	zLevels := min(pair.ZBitStringLen, levels)
	xyLevels := levels - zLevels

	return RawBitStringPair{
		RawXYBitString: pair.RawXYBitString.Ancestor(xyLevels),
		RawZBitString:  pair.RawZBitString.Ancestor(zLevels),
	}
}

// shorthand for .Ancestor(1)
func (pair *RawXYBitString) Parent() RawXYBitString {
	return pair.Ancestor(1)
}

// shorthand for .Ancestor(1)
func (pair *RawZBitString) Parent() RawZBitString {
	return pair.Ancestor(1)
}

// shorthand for .AncestorPair(1)
func (pair *RawBitStringPair) ParentPair() RawBitStringPair {
	return pair.AncestorPair(1)
}

// returns the neighbor
func (pair *RawXYBitString) Neighbor() RawXYBitString {
	return RawXYBitString{
		// xor with 1 flips a bit, xor with 0 keeps the same value
		XYBitString:    pair.XYBitString ^ (1 << (64 - pair.XYBitStringLen)),
		XYBitStringLen: pair.XYBitStringLen,
	}
}

// returns the neighbor
func (pair *RawZBitString) Neighbor() RawZBitString {
	return RawZBitString{
		// xor with 1 flips a bit, xor with 0 keeps the same value
		ZBitString:    pair.ZBitString ^ (1 << (16 - pair.ZBitStringLen)),
		ZBitStringLen: pair.ZBitStringLen,
	}
}

// returns XYBitString and ZBitString of the neighbor as a pair struct
func (pair *RawBitStringPair) NeighborPair() RawBitStringPair {
	if pair.ZBitStringLen == 0 {
		// flip last bit of the xy bit string
		return RawBitStringPair{
			RawXYBitString: pair.RawXYBitString.Neighbor(),
			RawZBitString:  pair.RawZBitString,
		}
	}

	// flip last bit of the z bit string
	return RawBitStringPair{
		RawXYBitString: pair.RawXYBitString,
		RawZBitString:  pair.RawZBitString.Neighbor(),
	}
}

// returns the left child
func (pair *RawXYBitString) LeftChild() RawXYBitString {
	return RawXYBitString{
		// clear all bits except the used bits
		XYBitString: pair.XYBitString & (uint64(math.MaxUint64) << (64 - pair.XYBitStringLen)),
		// then extend the length, this now includes one of the cleared bits
		XYBitStringLen: pair.XYBitStringLen + 1,
	}
}

// returns XYBitString and ZBitString of the xyLeftChild as a pair struct
func (pair *RawBitStringPair) XYLeftChildPair() (RawBitStringPair, error) {
	if pair.ZBitStringLen > 0 {
		return RawBitStringPair{}, fmt.Errorf("cannot call .XYLeftChildPair() on non 2D bit string")
	}

	return RawBitStringPair{
		RawXYBitString: pair.RawXYBitString.LeftChild(),
		RawZBitString:  pair.RawZBitString,
	}, nil
}

// returns the right child
func (pair *RawXYBitString) RightChild() RawXYBitString {
	return RawXYBitString{
		// set the first not used bit
		XYBitString: pair.XYBitString | (1 << (64 - pair.XYBitStringLen - 1)),
		// then extend the length, this now includes one of the set bits
		XYBitStringLen: pair.XYBitStringLen + 1,
	}
}

// returns XYBitString and ZBitString of the xyRightChild as a pair struct
func (pair *RawBitStringPair) XYRightChildPair() (RawBitStringPair, error) {
	if pair.ZBitStringLen > 0 {
		return RawBitStringPair{}, fmt.Errorf("cannot call .XYRightChildPair() on non 2D bit string")
	}

	return RawBitStringPair{
		RawXYBitString: pair.RawXYBitString.RightChild(),
		RawZBitString:  pair.RawZBitString,
	}, nil
}

// returns the left child
func (pair *RawZBitString) LeftChild() RawZBitString {
	return RawZBitString{
		// clear all bits except the used bits
		ZBitString: pair.ZBitString & (uint16(math.MaxUint16) << (16 - pair.ZBitStringLen)),
		// then extend the length, this now includes one of the cleared bits
		ZBitStringLen: pair.ZBitStringLen + 1,
	}
}

// returns XYBitString and ZBitString of the zLeftChild as a pair struct
func (pair *RawBitStringPair) ZLeftChildPair() RawBitStringPair {
	return RawBitStringPair{
		RawXYBitString: pair.RawXYBitString,
		RawZBitString:  pair.RawZBitString.LeftChild(),
	}
}

// returns the right child
func (pair *RawZBitString) RightChild() RawZBitString {
	return RawZBitString{
		// set the first not used bit
		ZBitString: pair.ZBitString | (1 << (16 - pair.ZBitStringLen - 1)),
		// then extend the length, this now includes one of the set bits
		ZBitStringLen: pair.ZBitStringLen + 1,
	}
}

// returns XYBitString and ZBitString of the zRightChild as a pair struct
func (pair *RawBitStringPair) ZRightChildPair() RawBitStringPair {
	return RawBitStringPair{
		RawXYBitString: pair.RawXYBitString,
		RawZBitString:  pair.RawZBitString.RightChild(),
	}
}
