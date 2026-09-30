package crypto

import "testing"

// TestCompleteness tests that the client-side completeness checks reject responses
// where the server omitted a non-empty z subtree intersecting the queried altitude range,
// and accept responses where the omission is legitimate (empty subtree or no intersection).
//
// z ranges (Z_BITS = 15, upper bound exclusive):
//   - xy node (z len 0):      children [0, 16384) and [16384, 32768)
//   - z node 0x0000 (len 1):  [0, 16384),     children [0, 8192) and [8192, 16384)
//   - z node 0x8000 (len 1):  [16384, 32768), children [16384, 24576) and [24576, 32768)
func TestCompleteness(t *testing.T) {
	nonEmpty := &SHA256Hash{1}

	leftOmittedRightPresent := newZNode(0x0000, 1, nonEmpty, nil)
	leftOmittedRightPresent.zRightChild = newZNode(0x4000, 2, nil, nil)

	tests := []struct {
		name                     string
		node                     *Node
		isComplete               func(*Node, int16, int16) bool
		minAltitude, maxAltitude int16
		want                     bool
	}{
		{
			name:        "z: omitted right child, query inside its range",
			node:        newZNode(0x0000, 1, nil, nonEmpty),
			isComplete:  (*Node).IsZComplete,
			minAltitude: 9000, maxAltitude: 10000,
			want: false,
		},
		{
			name:        "z: omitted right child, query outside its range",
			node:        newZNode(0x0000, 1, nil, nonEmpty),
			isComplete:  (*Node).IsZComplete,
			minAltitude: 1000, maxAltitude: 2000,
			want: true,
		},
		{
			name:        "z: omitted empty right child, query inside its range",
			node:        newZNode(0x0000, 1, nil, nil),
			isComplete:  (*Node).IsZComplete,
			minAltitude: 9000, maxAltitude: 10000,
			want: true,
		},
		{
			name:        "z: omitted left child, complete right child",
			node:        leftOmittedRightPresent,
			isComplete:  (*Node).IsZComplete,
			minAltitude: 0, maxAltitude: 16000,
			want: false,
		},
		{
			name:        "z: omitted top-most child, query inside its range",
			node:        newZNode(0x8000, 1, nil, nonEmpty),
			isComplete:  (*Node).IsZComplete,
			minAltitude: 25000, maxAltitude: 26000,
			want: false,
		},
		{
			name:        "z: omitted top-most child, query below its range",
			node:        newZNode(0x8000, 1, nil, nonEmpty),
			isComplete:  (*Node).IsZComplete,
			minAltitude: 17000, maxAltitude: 18000,
			want: true,
		},
		{
			name:        "xy: omitted lower z child, query inside its range",
			node:        newZNode(0x0000, 0, nonEmpty, nil),
			isComplete:  (*Node).IsComplete,
			minAltitude: 1000, maxAltitude: 2000,
			want: false,
		},
		{
			name:        "xy: omitted upper z child, query inside its range",
			node:        newZNode(0x0000, 0, nil, nonEmpty),
			isComplete:  (*Node).IsComplete,
			minAltitude: 17000, maxAltitude: 18000,
			want: false,
		},
		{
			name:        "xy: omitted upper z child, query below its range",
			node:        newZNode(0x0000, 0, nil, nonEmpty),
			isComplete:  (*Node).IsComplete,
			minAltitude: 1000, maxAltitude: 2000,
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.isComplete(tc.node, tc.minAltitude, tc.maxAltitude)
			if got != tc.want {
				t.Fatalf("query [%d, %d]: got complete=%v, want %v",
					tc.minAltitude, tc.maxAltitude, got, tc.want)
			}
		})
	}
}

// newZNode builds a node without xy children at the given raw z bit string,
// with the given z child hashes (nil = empty subtree)
func newZNode(zBitString uint16, zBitStringLen uint8, zLeftChildHash, zRightChildHash *SHA256Hash) *Node {
	return NewNode(0, 0, zBitString, zBitStringLen, nil, nil, zLeftChildHash, zRightChildHash, nil)
}
