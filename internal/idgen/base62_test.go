package idgen

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBase62Encode(t *testing.T) {
	tests := []struct {
		name     string
		input    uint64
		expected string
	}{
		{
			name:     "zero",
			input:    0,
			expected: "0",
		},
		{
			name:     "single digit",
			input:    9,
			expected: "9",
		},
		{
			name:     "ten encodes to a",
			input:    10,
			expected: "a",
		},
		{
			name:     "35 encodes to z",
			input:    35,
			expected: "z",
		},
		{
			name:     "36 encodes to A",
			input:    36,
			expected: "A",
		},
		{
			name:     "61 encodes to Z",
			input:    61,
			expected: "Z",
		},
		{
			name:     "62 encodes to 10",
			input:    62,
			expected: "10",
		},
		{
			name:     "large number",
			input:    238328,
			expected: "1000", // 62^3 = 238328
		},
		{
			name:     "max uint64",
			input:    18446744073709551615,
			expected: "lYGhA16ahyf", // 62-based encoding of max uint64
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Encode(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBase62Decode(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  uint64
		expectErr bool
	}{
		{
			name:     "zero",
			input:    "0",
			expected: 0,
		},
		{
			name:     "single digit",
			input:    "9",
			expected: 9,
		},
		{
			name:     "lowercase a",
			input:    "a",
			expected: 10,
		},
		{
			name:     "lowercase z",
			input:    "z",
			expected: 35,
		},
		{
			name:     "uppercase A",
			input:    "A",
			expected: 36,
		},
		{
			name:     "uppercase Z",
			input:    "Z",
			expected: 61,
		},
		{
			name:     "multi char",
			input:    "10",
			expected: 62,
		},
		{
			name:     "large number",
			input:    "1000",
			expected: 238328,
		},
		{
			name:      "invalid character underscore",
			input:     "abc_def",
			expectErr: true,
		},
		{
			name:      "invalid character hyphen",
			input:     "abc-def",
			expectErr: true,
		},
		{
			name:      "empty string",
			input:     "",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Decode(tt.input)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestBase62RoundTrip(t *testing.T) {
	// Test that encoding and decoding are inverse operations
	testValues := []uint64{
		0, 1, 10, 61, 62, 100, 1000, 10000, 100000,
		1000000, 238328, 14776336, 916132832,
	}

	for _, val := range testValues {
		encoded := Encode(val)
		decoded, err := Decode(encoded)
		require.NoError(t, err, "failed to decode %s (original: %d)", encoded, val)
		assert.Equal(t, val, decoded, "round trip failed for %d", val)
	}
}

func TestBase62EncodeWithPadding(t *testing.T) {
	tests := []struct {
		name      string
		input     uint64
		minLength int
		expected  string
	}{
		{
			name:      "zero with padding 6",
			input:     0,
			minLength: 6,
			expected:  "000000",
		},
		{
			name:      "small number padded",
			input:     1,
			minLength: 4,
			expected:  "0001",
		},
		{
			name:      "no padding needed",
			input:     238328,
			minLength: 4,
			expected:  "1000",
		},
		{
			name:      "minLength 0 returns normal",
			input:     62,
			minLength: 0,
			expected:  "10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EncodeWithPadding(tt.input, tt.minLength)
			assert.Equal(t, tt.expected, result)
			assert.GreaterOrEqual(t, len(result), tt.minLength)
		})
	}
}

func BenchmarkBase62Encode(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Encode(uint64(i))
	}
}

func BenchmarkBase62Decode(b *testing.B) {
	encoded := Encode(238328)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Decode(encoded)
	}
}
