package bitstring

import (
	"strconv"
	"strings"
	"testing"
)

func TestXYBitString(t *testing.T) {

	xyBitString := "010011010001"
	rightPaddedXYBitString := xyBitString + strings.Repeat("0", 64-len(xyBitString))

	parsedXYBitString, err := strconv.ParseUint(rightPaddedXYBitString, 2, 64)
	if err != nil {
		t.Fatalf(`should not throw error: %v`, err)
	}

	b := RawXYBitString{
		XYBitString:    parsedXYBitString,
		XYBitStringLen: uint8(len(xyBitString)),
	}

	if b.BitString().String() != xyBitString {
		t.Fatalf(`invalid bit string, expected %s, received %s`, xyBitString, b.BitString().String())
	}
}
