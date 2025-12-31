package idgen

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRandomGenerator_Generate(t *testing.T) {
	t.Run("generates code of correct length", func(t *testing.T) {
		gen := NewRandomGenerator(7)
		code, err := gen.Generate()
		require.NoError(t, err)
		assert.Len(t, code, 7)
	})

	t.Run("generates valid base62 codes", func(t *testing.T) {
		gen := NewRandomGenerator(8)
		for i := 0; i < 100; i++ {
			code, err := gen.Generate()
			require.NoError(t, err)
			assert.True(t, IsValid(code), "code %q should be valid base62", code)
		}
	})

	t.Run("generates unique codes", func(t *testing.T) {
		gen := NewRandomGenerator(6)
		seen := make(map[string]bool)
		numCodes := 10000

		for i := 0; i < numCodes; i++ {
			code, err := gen.Generate()
			require.NoError(t, err)
			assert.False(t, seen[code], "duplicate code generated: %s", code)
			seen[code] = true
		}
	})

	t.Run("concurrent generation is safe", func(t *testing.T) {
		gen := NewRandomGenerator(8)
		numGoroutines := 100
		codesPerGoroutine := 100

		var wg sync.WaitGroup
		var mu sync.Mutex
		seen := make(map[string]bool)
		errors := make([]error, 0)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < codesPerGoroutine; j++ {
					code, err := gen.Generate()
					mu.Lock()
					if err != nil {
						errors = append(errors, err)
					} else {
						seen[code] = true
					}
					mu.Unlock()
				}
			}()
		}
		wg.Wait()

		assert.Empty(t, errors, "should have no errors")
		// With 8 characters, collision probability is extremely low
		assert.Greater(t, len(seen), numGoroutines*codesPerGoroutine*95/100,
			"should have generated mostly unique codes")
	})
}

func TestRandomGenerator_WithLength(t *testing.T) {
	tests := []struct {
		name   string
		length int
	}{
		{"minimum length 4", 4},
		{"default length 7", 7},
		{"longer length 10", 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gen := NewRandomGenerator(tt.length)
			code, err := gen.Generate()
			require.NoError(t, err)
			assert.Len(t, code, tt.length)
		})
	}
}

func TestDefaultGenerator(t *testing.T) {
	gen := NewDefaultGenerator()
	code, err := gen.Generate()
	require.NoError(t, err)
	assert.Len(t, code, DefaultCodeLength)
	assert.True(t, IsValid(code))
}

func TestGeneratorInterface(t *testing.T) {
	// Verify interface compliance at compile time
	var _ Generator = (*RandomGenerator)(nil)
	var _ Generator = (*SnowflakeGenerator)(nil)
}

func BenchmarkRandomGenerator_Generate(b *testing.B) {
	gen := NewRandomGenerator(7)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = gen.Generate()
	}
}

func BenchmarkRandomGenerator_ConcurrentGenerate(b *testing.B) {
	gen := NewRandomGenerator(7)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = gen.Generate()
		}
	})
}
