package main

import (
	"sync"
	"testing"
)

// Two goroutines each hammer their own counter; false sharing shows as a slowdown in the
// unpadded layout. Run: go test -bench . -cpu 2,4
func hammer(b *testing.B, a, c interface{ Add(uint64) uint64 }) {
	var wg sync.WaitGroup
	wg.Add(2)
	n := b.N
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			a.Add(1)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			c.Add(1)
		}
	}()
	wg.Wait()
}
func BenchmarkPadded(b *testing.B)   { var p padded; hammer(b, &p.head, &p.tail) }
func BenchmarkUnpadded(b *testing.B) { var u unpadded; hammer(b, &u.head, &u.tail) }
