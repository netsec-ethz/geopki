package bitstring

import "math"

type RawBitStringPair struct {
	// the bit string encoding the x and y coordinate
	//[7]byte interpreted as a big endian integer
	XYBitString uint64
	// the number of most siginifanct / first bits that are used
	// has to be in [0, 52]
	XYBitStringLen uint8

	// the bit string encoding the z coordinate
	//[2]byte interpreted as a big endian integer
	ZBitString uint16
	// the number of most siginifanct / first bits that are used
	// has to be in [0, 15]
	ZBitStringLen uint8
}

// returns XYBitString and ZBitString of the neighbor as a pair struct
func (pair RawBitStringPair) NeighborPair() RawBitStringPair {
	if pair.ZBitStringLen == 0 {
		// flip last bit of the xy bit string
		return RawBitStringPair{
			// xor with 1 flips a bit, xor with 0 keeps the same value
			XYBitString:    pair.XYBitString ^ (1 << (64 - pair.XYBitStringLen)),
			XYBitStringLen: pair.XYBitStringLen,
			ZBitString:     pair.ZBitString,
			ZBitStringLen:  0,
		}
	}

	// flip last bit of the z bit string
	return RawBitStringPair{
		XYBitString:    pair.XYBitString,
		XYBitStringLen: pair.XYBitStringLen,
		// xor with 1 flips a bit, xor with 0 keeps the same value
		ZBitString:    pair.ZBitString ^ (1 << (16 - pair.ZBitString)),
		ZBitStringLen: 0,
	}
}

// returns XYBitString and ZBitString of the xyLeftChild as a pair struct
func (pair RawBitStringPair) XYLeftChildPair() RawBitStringPair {
	return RawBitStringPair{
		// clear all bits except the used bits
		XYBitString: pair.XYBitString & (math.MaxUint64 << (64 - pair.XYBitStringLen)),
		// then extend the length, this now includes one of the cleared bits
		XYBitStringLen: pair.XYBitStringLen + 1,
		ZBitString:     pair.ZBitString,
		ZBitStringLen:  pair.ZBitStringLen,
	}
}

// returns XYBitString and ZBitString of the xyRightChild as a pair struct
func (pair RawBitStringPair) XYRightChildPair() RawBitStringPair {
	return RawBitStringPair{
		// set the first not used bit
		XYBitString: pair.XYBitString | (1 << (64 - pair.XYBitStringLen - 1)),
		// then extend the length, this now includes one of the set bits
		XYBitStringLen: pair.XYBitStringLen + 1,
		ZBitString:     pair.ZBitString,
		ZBitStringLen:  pair.ZBitStringLen,
	}
}

// returns XYBitString and ZBitString of the zLeftChild as a pair struct
func (pair RawBitStringPair) ZLeftChildPair() RawBitStringPair {
	return RawBitStringPair{
		XYBitString:    pair.XYBitString,
		XYBitStringLen: pair.XYBitStringLen,
		// clear all bits except the used bits
		ZBitString: pair.ZBitString & (math.MaxUint16 << (64 - pair.ZBitStringLen)),
		// then extend the length, this now includes one of the cleared bits
		ZBitStringLen: pair.ZBitStringLen + 1,
	}
}

// returns XYBitString and ZBitString of the zRightChild as a pair struct
func (pair RawBitStringPair) ZRightChildPair() RawBitStringPair {
	return RawBitStringPair{
		XYBitString:    pair.XYBitString,
		XYBitStringLen: pair.XYBitStringLen,
		// set the first not used bit
		ZBitString: pair.ZBitString | (1 << (16 - pair.ZBitStringLen - 1)),
		// then extend the length, this now includes one of the set bits
		ZBitStringLen: pair.ZBitStringLen + 1,
	}
}
