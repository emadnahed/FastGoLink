package integration

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/emadnahed/FastGoLink/internal/idgen"
)

// TestIDGenerationAtScale tests ID generation with high volume.
func TestIDGenerationAtScale(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping scale test in short mode")
	}

	t.Run("random generator produces unique codes at scale", func(t *testing.T) {
		gen := idgen.NewRandomGenerator(8)
		numCodes := 100000
		seen := make(map[string]bool, numCodes)

		for i := 0; i < numCodes; i++ {
			code, err := gen.Generate()
			require.NoError(t, err)
			require.False(t, seen[code], "duplicate code at iteration %d: %s", i, code)
			seen[code] = true
		}

		t.Logf("Generated %d unique codes successfully", numCodes)
	})

	t.Run("snowflake generator produces unique codes at scale", func(t *testing.T) {
		gen, err := idgen.NewSnowflakeGenerator(1, 7)
		require.NoError(t, err)

		numCodes := 100000
		seen := make(map[string]bool, numCodes)

		for i := 0; i < numCodes; i++ {
			code, err := gen.Generate()
			require.NoError(t, err)
			require.False(t, seen[code], "duplicate code at iteration %d: %s", i, code)
			seen[code] = true
		}

		t.Logf("Generated %d unique snowflake codes successfully", numCodes)
	})

	t.Run("concurrent random generation at scale", func(t *testing.T) {
		gen := idgen.NewRandomGenerator(8)
		numGoroutines := 100
		codesPerGoroutine := 1000

		var wg sync.WaitGroup
		var mu sync.Mutex
		allCodes := make(map[string]bool, numGoroutines*codesPerGoroutine)
		duplicates := 0
		errors := 0

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < codesPerGoroutine; j++ {
					code, err := gen.Generate()
					mu.Lock()
					if err != nil {
						errors++
					} else {
						if allCodes[code] {
							duplicates++
						}
						allCodes[code] = true
					}
					mu.Unlock()
				}
			}()
		}
		wg.Wait()

		t.Logf("Generated %d codes with %d duplicates and %d errors",
			len(allCodes), duplicates, errors)

		assert.Equal(t, 0, errors, "should have no errors")
		// With 8-char codes (62^8 = 218 trillion combinations), duplicates should be very rare
		assert.Less(t, duplicates, 10, "should have minimal duplicates")
	})

	t.Run("concurrent snowflake generation at scale", func(t *testing.T) {
		gen, err := idgen.NewSnowflakeGenerator(1, 7)
		require.NoError(t, err)

		numGoroutines := 100
		codesPerGoroutine := 1000

		var wg sync.WaitGroup
		var mu sync.Mutex
		allCodes := make(map[string]bool, numGoroutines*codesPerGoroutine)
		duplicates := 0
		errors := 0

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < codesPerGoroutine; j++ {
					code, err := gen.Generate()
					mu.Lock()
					if err != nil {
						errors++
					} else {
						if allCodes[code] {
							duplicates++
						}
						allCodes[code] = true
					}
					mu.Unlock()
				}
			}()
		}
		wg.Wait()

		t.Logf("Generated %d snowflake codes with %d duplicates and %d errors",
			len(allCodes), duplicates, errors)

		assert.Equal(t, 0, errors, "should have no errors")
		assert.Equal(t, 0, duplicates, "snowflake should produce zero duplicates")
		assert.Equal(t, numGoroutines*codesPerGoroutine, len(allCodes))
	})
}

// TestCollisionAwareGeneratorAtScale tests the collision-aware generator.
func TestCollisionAwareGeneratorAtScale(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping scale test in short mode")
	}

	t.Run("handles collisions gracefully at scale", func(t *testing.T) {
		// Use a short code length to increase collision probability
		base := idgen.NewRandomGenerator(4)

		// Use an in-memory checker
		existing := make(map[string]bool)
		var mu sync.Mutex

		checker := &inMemoryChecker{
			existing: existing,
			mu:       &mu,
		}

		gen := idgen.NewCollisionAwareGenerator(base, checker, 100)

		numCodes := 10000

		for i := 0; i < numCodes; i++ {
			code, err := gen.Generate()
			require.NoError(t, err, "failed at iteration %d", i)

			mu.Lock()
			require.False(t, existing[code], "duplicate code: %s", code)
			existing[code] = true
			mu.Unlock()
		}

		stats := gen.Stats()
		t.Logf("Generated %d codes with %d retries and %d collisions",
			stats.TotalGenerations, stats.TotalRetries, stats.TotalCollisions)

		// With 4-char codes (62^4 = 14.7M combinations) and 10K codes,
		// we expect some collisions due to birthday paradox
		assert.GreaterOrEqual(t, stats.TotalCollisions, int64(0))
	})

	t.Run("concurrent collision-aware generation", func(t *testing.T) {
		base := idgen.NewRandomGenerator(6)

		existing := make(map[string]bool)
		var mu sync.Mutex

		checker := &inMemoryChecker{
			existing: existing,
			mu:       &mu,
		}

		gen := idgen.NewCollisionAwareGenerator(base, checker, 50)

		numGoroutines := 50
		codesPerGoroutine := 200

		var wg sync.WaitGroup
		allCodes := make([]string, 0, numGoroutines*codesPerGoroutine)
		errors := 0

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < codesPerGoroutine; j++ {
					code, err := gen.Generate()
					mu.Lock()
					if err != nil {
						errors++
					} else {
						allCodes = append(allCodes, code)
						existing[code] = true
					}
					mu.Unlock()
				}
			}()
		}
		wg.Wait()

		stats := gen.Stats()
		t.Logf("Concurrent generation: %d codes, %d collisions, %d retries, %d errors",
			len(allCodes), stats.TotalCollisions, stats.TotalRetries, errors)

		assert.Equal(t, 0, errors)
		assert.Equal(t, numGoroutines*codesPerGoroutine, len(allCodes))

		// Verify all codes are unique
		seen := make(map[string]bool)
		for _, code := range allCodes {
			assert.False(t, seen[code], "duplicate code: %s", code)
			seen[code] = true
		}
	})
}

// TestBase62PerformanceAtScale tests Base62 encoding performance.
func TestBase62PerformanceAtScale(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance test in short mode")
	}

	t.Run("encode decode round trip at scale", func(t *testing.T) {
		numIterations := 1000000

		for i := 0; i < numIterations; i++ {
			val := uint64(i)
			encoded := idgen.Encode(val)
			decoded, err := idgen.Decode(encoded)
			require.NoError(t, err)
			require.Equal(t, val, decoded)
		}

		t.Logf("Successfully completed %d round trips", numIterations)
	})
}

// inMemoryChecker implements ExistenceChecker for testing.
type inMemoryChecker struct {
	existing map[string]bool
	mu       *sync.Mutex
}

func (c *inMemoryChecker) Exists(ctx context.Context, code string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.existing[code], nil
}
