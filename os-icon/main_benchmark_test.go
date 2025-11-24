package main

/*
Benchmark Results Summary:

Fast Implementation (getLinuxDistroIDFast):
- 2,234,356 ops/sec (550.7 ns/op)
- Memory: 232 B/op, 2 allocs/op

Slow Implementation (getLinuxDistroID):
- 102,259 ops/sec (11,943 ns/op)
- Memory: 14,720 B/op, 120 allocs/op

Performance improvements:
- 21.7x faster execution speed
- 63.4x less memory usage
- 60x fewer memory allocations

*/

import (
	"testing"
)

// BenchmarkGetLinuxDistroIDFast benchmarks the fast implementation
func BenchmarkGetLinuxDistroIDFast(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = getLinuxDistroIDFast()
	}
}

// BenchmarkGetLinuxDistroIDSlow benchmarks only the slow INI parser implementation
func BenchmarkGetLinuxDistroIDSlow(b *testing.B) {
	for i := 0; i < b.N; i++ {
		// This mimics the original slow implementation
		_ = getLinuxDistroID()
	}
}
