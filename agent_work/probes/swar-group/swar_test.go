// Probe (Phase 1, Fable): 16-slot SwissTable group matching in pure Go — SWAR over two
// uint64 words vs a scalar byte loop. Checks (a) SWAR never misses a true H2 match,
// (b) how often it reports a false positive, (c) empty-match exactness, (d) relative speed.
// Run: go test -run . -bench . -benchmem
package swar

import (
	"encoding/binary"
	"math/bits"
	"math/rand"
	"testing"
)

const (
	lsb   = 0x0101010101010101
	msb   = 0x8080808080808080
	empty = 0x80
	tomb  = 0xFE
)

func matchH2(w uint64, h2 byte) uint64 { x := w ^ (lsb * uint64(h2)); return (x - lsb) &^ x & msb }
func matchEmpty(w uint64) uint64       { return (w &^ (w << 6)) & msb }

func swar16(g *[16]byte, h2 byte) uint32 {
	lo, hi := binary.LittleEndian.Uint64(g[:8]), binary.LittleEndian.Uint64(g[8:])
	return pack(matchH2(lo, h2)) | pack(matchH2(hi, h2))<<8
}
func pack(m uint64) uint32 { // msb-per-byte mask -> 8-bit mask
	var out uint32
	for m != 0 {
		out |= 1 << (bits.TrailingZeros64(m) >> 3)
		m &= m - 1
	}
	return out
}
func scalar16(g *[16]byte, h2 byte) uint32 {
	var out uint32
	for i, c := range g {
		if c == h2 {
			out |= 1 << i
		}
	}
	return out
}

func randGroup(r *rand.Rand) [16]byte {
	var g [16]byte
	for i := range g {
		switch r.Intn(10) {
		case 0:
			g[i] = empty
		case 1:
			g[i] = tomb
		default:
			g[i] = byte(r.Intn(128))
		}
	}
	return g
}

func TestSWARvsScalar(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	var missed, falsePos, emptyWrong int
	const n = 2_000_000
	for i := 0; i < n; i++ {
		g := randGroup(r)
		h2 := byte(r.Intn(128))
		s, w := scalar16(&g, h2), swar16(&g, h2)
		if s&^w != 0 {
			missed++
		}
		if w&^s != 0 {
			falsePos++
		}
		lo, hi := binary.LittleEndian.Uint64(g[:8]), binary.LittleEndian.Uint64(g[8:])
		if pack(matchEmpty(lo))|pack(matchEmpty(hi))<<8 != scalar16(&g, empty) {
			emptyWrong++
		}
	}
	t.Logf("groups=%d missed=%d falsePositiveGroups=%d (%.4f%%) emptyMismatch=%d", n, missed, falsePos, 100*float64(falsePos)/n, emptyWrong)
	if missed != 0 || emptyWrong != 0 {
		t.Fatal("SWAR unsound")
	}
}

var sink uint32

func bench(b *testing.B, f func(*[16]byte, byte) uint32) {
	r := rand.New(rand.NewSource(2))
	gs := make([][16]byte, 1024)
	for i := range gs {
		gs[i] = randGroup(r)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink += f(&gs[i&1023], byte(i&127))
	}
}
func BenchmarkSWAR16(b *testing.B)   { bench(b, swar16) }
func BenchmarkScalar16(b *testing.B) { bench(b, scalar16) }
