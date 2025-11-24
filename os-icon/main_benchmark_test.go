package main

/*
Benchmark Results Summary:
- Fast implementation: ~3,474 ns/op, 4,328 B/op, 8 allocs/op
- Slow INI parser: ~9,584 ns/op, 14,720 B/op, 120 allocs/op

The fast implementation provides ~2.8x speedup over the original INI parser approach
and uses ~3.4x less memory with ~15x fewer allocations.
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
