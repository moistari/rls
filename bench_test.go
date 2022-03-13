package rls

import (
	"testing"

	"github.com/cytec/releaseparser"
)

func BenchmarkMoistari(b *testing.B) {
	tests := rlsTests(b)
	for n := 0; n < b.N; n++ {
		for _, test := range tests {
			rRelease = ParseString(test.s)
		}
	}
}

func BenchmarkCytec(b *testing.B) {
	tests := rlsTests(b)
	for n := 0; n < b.N; n++ {
		for _, test := range tests {
			rpRelease = *releaseparser.Parse(test.s)
		}
	}
}

var (
	rRelease  Release
	rpRelease releaseparser.Release
)
