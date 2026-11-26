// Package procedural provides deterministic hashing, random numbers and noise
// functions used by every generator in Evergreen. Nothing in this package
// touches global state, so identical inputs always produce identical outputs.
package procedural

import (
	"hash/fnv"
	"strconv"
	"strings"
)

func mix64(z uint64) uint64 {
	z ^= z >> 30
	z *= 0xbf58476d1ce4e5b9
	z ^= z >> 27
	z *= 0x94d049bb133111eb
	z ^= z >> 31
	return z
}

// Hash2 hashes a seed and an integer lattice coordinate.
func Hash2(seed uint64, x, y int32) uint64 {
	return mix64(seed ^ uint64(uint32(x))*0x9E3779B97F4A7C15 ^ uint64(uint32(y))*0xC2B2AE3D27D4EB4F)
}

// Hash3 hashes a seed and a 3D lattice coordinate.
func Hash3(seed uint64, x, y, z int32) uint64 {
	return mix64(Hash2(seed, x, y) ^ uint64(uint32(z))*0x165667B19E3779F9)
}

// HashFloat2 returns a deterministic value in [0,1) for a lattice coordinate.
func HashFloat2(seed uint64, x, y int32) float64 {
	return float64(Hash2(seed, x, y)>>11) / (1 << 53)
}

// SeedFromString converts a user-facing seed into a 64-bit seed. Decimal
// strings map to their numeric value; any other text is hashed with FNV-1a.
func SeedFromString(s string) uint64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if n, err := strconv.ParseUint(s, 10, 64); err == nil {
		return n
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}

// RNG is a small, allocation-free SplitMix64 generator.
type RNG struct{ s uint64 }

// NewRNG returns a generator seeded with seed.
func NewRNG(seed uint64) *RNG { return &RNG{s: seed} }

// Next returns the next 64 random bits.
func (r *RNG) Next() uint64 {
	r.s += 0x9E3779B97F4A7C15
	return mix64(r.s)
}

// Float returns a value in [0,1).
func (r *RNG) Float() float64 { return float64(r.Next()>>11) / (1 << 53) }

// Range returns a value in [a,b).
func (r *RNG) Range(a, b float64) float64 { return a + (b-a)*r.Float() }

// Intn returns an int in [0,n). n must be > 0.
func (r *RNG) Intn(n int) int { return int(r.Next() % uint64(n)) }

// Chance reports true with probability p.
func (r *RNG) Chance(p float64) bool { return r.Float() < p }

// Sign returns -1 or 1.
func (r *RNG) Sign() float64 {
	if r.Next()&1 == 0 {
		return -1
	}
	return 1
}
