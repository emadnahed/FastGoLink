package idgen

import (
	"errors"
	"strings"
)

// Base62 alphabet: 0-9, a-z, A-Z (62 characters)
// This provides URL-safe, compact encoding for numeric IDs.
const alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

const base = 62

// ErrInvalidCharacter is returned when decoding encounters an invalid character.
var ErrInvalidCharacter = errors.New("invalid base62 character")

// ErrEmptyString is returned when decoding an empty string.
var ErrEmptyString = errors.New("cannot decode empty string")

// charToValue maps each character to its numeric value for fast decoding.
var charToValue [256]int

func init() {
	// Initialize all values to -1 (invalid)
	for i := range charToValue {
		charToValue[i] = -1
	}
	// Map valid characters to their values
	for i, c := range alphabet {
		charToValue[c] = i
	}
}

// Encode converts a uint64 to a Base62 string.
func Encode(n uint64) string {
	if n == 0 {
		return "0"
	}

	var result strings.Builder
	// Pre-allocate for efficiency (11 chars can hold max uint64)
	result.Grow(11)

	for n > 0 {
		remainder := n % base
		result.WriteByte(alphabet[remainder])
		n /= base
	}

	// Reverse the string (we built it backwards)
	return reverse(result.String())
}

// EncodeWithPadding converts a uint64 to a Base62 string with minimum length.
// If the encoded string is shorter than minLength, it's left-padded with zeros.
func EncodeWithPadding(n uint64, minLength int) string {
	encoded := Encode(n)
	if len(encoded) >= minLength {
		return encoded
	}
	// Pad with leading zeros
	return strings.Repeat("0", minLength-len(encoded)) + encoded
}

// Decode converts a Base62 string back to a uint64.
func Decode(s string) (uint64, error) {
	if len(s) == 0 {
		return 0, ErrEmptyString
	}

	var result uint64
	for i := 0; i < len(s); i++ {
		val := charToValue[s[i]]
		if val == -1 {
			return 0, ErrInvalidCharacter
		}
		// #nosec G115 -- val is always in range [0, 61] from charToValue lookup
		result = result*base + uint64(val)
	}

	return result, nil
}

// reverse returns the reverse of a string.
func reverse(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

// IsValid checks if a string contains only valid Base62 characters.
func IsValid(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if charToValue[s[i]] == -1 {
			return false
		}
	}
	return true
}
