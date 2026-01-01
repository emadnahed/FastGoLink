package idgen

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSnowflakeGenerator(t *testing.T) {
	t.Run("valid node ID 0", func(t *testing.T) {
		gen, err := NewSnowflakeGenerator(0, 7)
		require.NoError(t, err)
		assert.NotNil(t, gen)
		assert.Equal(t, int64(0), gen.NodeID())
	})

	t.Run("valid node ID max (1023)", func(t *testing.T) {
		gen, err := NewSnowflakeGenerator(1023, 7)
		require.NoError(t, err)
		assert.NotNil(t, gen)
		assert.Equal(t, int64(1023), gen.NodeID())
	})

	t.Run("invalid node ID negative", func(t *testing.T) {
		gen, err := NewSnowflakeGenerator(-1, 7)
		assert.ErrorIs(t, err, ErrInvalidNodeID)
		assert.Nil(t, gen)
	})

	t.Run("invalid node ID too large", func(t *testing.T) {
		gen, err := NewSnowflakeGenerator(1024, 7)
		assert.ErrorIs(t, err, ErrInvalidNodeID)
		assert.Nil(t, gen)
	})

	t.Run("default min length on invalid", func(t *testing.T) {
		gen, err := NewSnowflakeGenerator(1, 0)
		require.NoError(t, err)
		// Generate and verify it has at least DefaultCodeLength
		code, err := gen.Generate()
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(code), DefaultCodeLength)
	})
}

func TestSnowflakeGenerator_Generate(t *testing.T) {
	t.Run("generates valid base62 codes", func(t *testing.T) {
		gen, err := NewSnowflakeGenerator(1, 7)
		require.NoError(t, err)

		for i := 0; i < 100; i++ {
			code, err := gen.Generate()
			require.NoError(t, err)
			assert.True(t, IsValid(code), "code %q should be valid base62", code)
		}
	})

	t.Run("generates codes at least minLength", func(t *testing.T) {
		gen, err := NewSnowflakeGenerator(1, 10)
		require.NoError(t, err)

		for i := 0; i < 100; i++ {
			code, err := gen.Generate()
			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(code), 10)
		}
	})

	t.Run("generates monotonically increasing IDs", func(t *testing.T) {
		gen, err := NewSnowflakeGenerator(1, 7)
		require.NoError(t, err)

		var lastID uint64
		for i := 0; i < 1000; i++ {
			code, err := gen.Generate()
			require.NoError(t, err)
			currentID, err := Decode(code)
			require.NoError(t, err)
			assert.Greater(t, currentID, lastID, "IDs should be monotonically increasing")
			lastID = currentID
		}
	})

	t.Run("generates unique codes", func(t *testing.T) {
		gen, err := NewSnowflakeGenerator(1, 7)
		require.NoError(t, err)

		seen := make(map[string]bool)
		numCodes := 10000

		for i := 0; i < numCodes; i++ {
			code, err := gen.Generate()
			require.NoError(t, err)
			assert.False(t, seen[code], "duplicate code generated: %s", code)
			seen[code] = true
		}
	})

	t.Run("concurrent generation produces unique codes", func(t *testing.T) {
		gen, err := NewSnowflakeGenerator(1, 7)
		require.NoError(t, err)

		numGoroutines := 100
		codesPerGoroutine := 100

		var wg sync.WaitGroup
		var mu sync.Mutex
		seen := make(map[string]bool)
		duplicates := 0

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < codesPerGoroutine; j++ {
					code, err := gen.Generate()
					if err != nil {
						continue
					}
					mu.Lock()
					if seen[code] {
						duplicates++
					}
					seen[code] = true
					mu.Unlock()
				}
			}()
		}
		wg.Wait()

		// Snowflake should produce zero duplicates
		assert.Equal(t, 0, duplicates, "snowflake should produce no duplicates")
		assert.Equal(t, numGoroutines*codesPerGoroutine, len(seen))
	})
}

func TestSnowflakeGenerator_DifferentNodes(t *testing.T) {
	// Different nodes should produce different IDs even at same time
	gen1, err := NewSnowflakeGenerator(1, 7)
	require.NoError(t, err)
	gen2, err := NewSnowflakeGenerator(2, 7)
	require.NoError(t, err)

	seen := make(map[string]bool)

	for i := 0; i < 1000; i++ {
		code1, err := gen1.Generate()
		require.NoError(t, err)
		code2, err := gen2.Generate()
		require.NoError(t, err)

		assert.False(t, seen[code1], "duplicate from gen1: %s", code1)
		assert.False(t, seen[code2], "duplicate from gen2: %s", code2)
		assert.NotEqual(t, code1, code2, "different nodes produced same code")

		seen[code1] = true
		seen[code2] = true
	}
}

func BenchmarkSnowflakeGenerator_Generate(b *testing.B) {
	gen, _ := NewSnowflakeGenerator(1, 7)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = gen.Generate()
	}
}

func BenchmarkSnowflakeGenerator_ConcurrentGenerate(b *testing.B) {
	gen, _ := NewSnowflakeGenerator(1, 7)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = gen.Generate()
		}
	})
}

func TestSnowflakeGenerator_ClockMovedBackwards(t *testing.T) {
	t.Run("returns error when clock moves backwards", func(t *testing.T) {
		gen, err := NewSnowflakeGenerator(1, 7)
		require.NoError(t, err)

		// Generate first code to set lastTime
		_, err = gen.Generate()
		require.NoError(t, err)

		// Simulate clock moving backwards by setting lastTime to far future
		gen.mu.Lock()
		gen.lastTime = gen.lastTime + 100000 // Set to future
		gen.mu.Unlock()

		// Next generate should fail
		code, err := gen.Generate()
		assert.ErrorIs(t, err, ErrClockMovedBackwards)
		assert.Empty(t, code)
	})
}

func TestSnowflakeGenerator_SequenceOverflow(t *testing.T) {
	t.Run("handles sequence overflow by waiting for next millisecond", func(t *testing.T) {
		gen, err := NewSnowflakeGenerator(1, 7)
		require.NoError(t, err)

		// Generate first code
		code1, err := gen.Generate()
		require.NoError(t, err)

		// Set sequence to near max to trigger overflow on next call
		gen.mu.Lock()
		gen.sequence = maxSequence
		gen.mu.Unlock()

		// Next generate should handle overflow and still succeed
		code2, err := gen.Generate()
		require.NoError(t, err)
		assert.NotEqual(t, code1, code2)
	})
}
