// Probe (Phase 1, Fable): does [N]byte padding around sync/atomic typed counters put the
// producer and consumer cursors on separate cache lines, and what are the struct offsets?
// Usage: go run .   (also cross-compiles: GOOS=darwin GOARCH=arm64 go build .)
package main

import (
	"fmt"
	"sync/atomic"
	"unsafe"
)

const line = 64

type padded struct {
	_    [line]byte
	head atomic.Uint64
	_    [line - 8]byte
	tail atomic.Uint64
	_    [line - 8]byte
	mask uint64
	buf  []uint64
}

type unpadded struct {
	head atomic.Uint64
	tail atomic.Uint64
}

func main() {
	var p padded
	var u unpadded
	fmt.Printf("padded:   head@%d tail@%d mask@%d size=%d align=%d\n",
		unsafe.Offsetof(p.head), unsafe.Offsetof(p.tail), unsafe.Offsetof(p.mask), unsafe.Sizeof(p), unsafe.Alignof(p))
	fmt.Printf("unpadded: head@%d tail@%d size=%d\n", unsafe.Offsetof(u.head), unsafe.Offsetof(u.tail), unsafe.Sizeof(u))
	fmt.Printf("head/tail on different 64B lines (any base alignment): %v\n",
		unsafe.Offsetof(p.tail)-unsafe.Offsetof(p.head) >= line)
	fmt.Printf("head/tail on different 128B lines (Apple silicon):     %v\n",
		unsafe.Offsetof(p.tail)-unsafe.Offsetof(p.head) >= 128)
}
