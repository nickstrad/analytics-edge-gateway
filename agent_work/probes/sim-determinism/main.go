// Probe (Phase 1, Fable): is a single-seed *math/rand.Rand workload stream bit-identical
// across runs and across goroutine-free replays, and what does rand.NewZipf accept?
// Usage: go run . [-seed N]   -> prints a SHA-256 of 1e6 mixed draws; run twice and compare.
package main

import (
	"crypto/sha256"
	"encoding/binary"
	"flag"
	"fmt"
	"math/rand"
)

func stream(seed int64) [32]byte {
	r := rand.New(rand.NewSource(seed))
	z := rand.NewZipf(r, 1.1, 1, 1<<20) // hot-key generator, s must be > 1
	h := sha256.New()
	var b [8]byte
	var now int64 // virtual nanoseconds
	for i := 0; i < 1_000_000; i++ {
		now += r.Int63n(1000)    // virtual clock advance
		k := z.Uint64()          // zipf key
		burst := r.Intn(100) < 3 // burst decision
		hc := r.Uint64()         // high-cardinality key
		for _, v := range []uint64{uint64(now), k, hc} {
			binary.LittleEndian.PutUint64(b[:], v)
			h.Write(b[:])
		}
		if burst {
			h.Write([]byte{1})
		}
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

func main() {
	seed := flag.Int64("seed", 42, "seed")
	flag.Parse()
	a, b := stream(*seed), stream(*seed)
	c := stream(*seed + 1)
	fmt.Printf("seed=%d digest=%x\n", *seed, a[:8])
	fmt.Printf("same-seed replay identical: %v\n", a == b)
	fmt.Printf("different seed differs:     %v\n", a != c)
	fmt.Printf("NewZipf(s=1.0) returns nil: %v (stdlib Zipf needs s>1)\n", rand.NewZipf(rand.New(rand.NewSource(1)), 1.0, 1, 10) == nil)
}
