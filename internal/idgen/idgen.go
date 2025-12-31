// Package idgen handles unique ID generation for short URLs.
package idgen

import (
	"crypto/rand"
	"math/big"
)

// DefaultCodeLength is the default length for generated short codes.
const DefaultCodeLength = 7

// Generator defines the interface for generating unique short codes.
type Generator interface {
	// Generate creates a new unique short code.
	Generate() (string, error)
}

// RandomGenerator generates random Base62 short codes.
type RandomGenerator struct {
	length int
}

// NewRandomGenerator creates a new RandomGenerator with the specified code length.
func NewRandomGenerator(length int) *RandomGenerator {
	if length < 1 {
		length = DefaultCodeLength
	}
	return &RandomGenerator{length: length}
}

// NewDefaultGenerator creates a RandomGenerator with the default code length.
func NewDefaultGenerator() *RandomGenerator {
	return NewRandomGenerator(DefaultCodeLength)
}

// Generate creates a new random Base62 short code.
// Uses crypto/rand for cryptographically secure randomness.
func (g *RandomGenerator) Generate() (string, error) {
	result := make([]byte, g.length)
	max := big.NewInt(int64(len(alphabet)))

	for i := 0; i < g.length; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		result[i] = alphabet[n.Int64()]
	}

	return string(result), nil
}

// Length returns the configured code length.
func (g *RandomGenerator) Length() int {
	return g.length
}
