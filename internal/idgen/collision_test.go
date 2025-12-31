package idgen

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockExistenceChecker simulates checking if a code exists in storage.
type mockExistenceChecker struct {
	mu       sync.RWMutex
	existing map[string]bool
}

func newMockExistenceChecker() *mockExistenceChecker {
	return &mockExistenceChecker{
		existing: make(map[string]bool),
	}
}

func (m *mockExistenceChecker) Exists(ctx context.Context, code string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.existing[code], nil
}

func (m *mockExistenceChecker) Add(code string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.existing[code] = true
}

// alwaysExistsChecker always returns true (always collides).
type alwaysExistsChecker struct{}

func (a *alwaysExistsChecker) Exists(ctx context.Context, code string) (bool, error) {
	return true, nil
}

// neverExistsChecker always returns false (never collides).
type neverExistsChecker struct{}

func (n *neverExistsChecker) Exists(ctx context.Context, code string) (bool, error) {
	return false, nil
}

func TestCollisionAwareGenerator(t *testing.T) {
	t.Run("generates unique code on first try when no collision", func(t *testing.T) {
		base := NewRandomGenerator(7)
		checker := &neverExistsChecker{}
		gen := NewCollisionAwareGenerator(base, checker, 3)

		code, err := gen.Generate()
		require.NoError(t, err)
		assert.Len(t, code, 7)
		assert.True(t, IsValid(code))
	})

	t.Run("retries on collision and succeeds", func(t *testing.T) {
		checker := newMockExistenceChecker()
		// Pre-populate with some codes that will collide
		base := NewRandomGenerator(7)

		// Generate a code and mark it as existing
		firstCode, _ := base.Generate()
		checker.Add(firstCode)

		gen := NewCollisionAwareGenerator(base, checker, 10)
		code, err := gen.Generate()
		require.NoError(t, err)
		assert.True(t, IsValid(code))
		// The generated code should be different from the pre-existing one
		// (or same if extremely unlucky, but retry logic should handle it)
	})

	t.Run("fails after max retries exceeded", func(t *testing.T) {
		base := NewRandomGenerator(7)
		checker := &alwaysExistsChecker{}
		gen := NewCollisionAwareGenerator(base, checker, 5)

		code, err := gen.Generate()
		assert.ErrorIs(t, err, ErrMaxRetriesExceeded)
		assert.Empty(t, code)
	})

	t.Run("works with snowflake generator", func(t *testing.T) {
		base, err := NewSnowflakeGenerator(1, 7)
		require.NoError(t, err)
		checker := &neverExistsChecker{}
		gen := NewCollisionAwareGenerator(base, checker, 3)

		code, err := gen.Generate()
		require.NoError(t, err)
		assert.True(t, IsValid(code))
	})

	t.Run("tracks collision statistics", func(t *testing.T) {
		checker := newMockExistenceChecker()
		base := NewRandomGenerator(7)

		gen := NewCollisionAwareGenerator(base, checker, 10)

		// Generate several codes
		for i := 0; i < 10; i++ {
			code, err := gen.Generate()
			require.NoError(t, err)
			// Mark each generated code as existing for next iteration
			checker.Add(code)
		}

		stats := gen.Stats()
		assert.GreaterOrEqual(t, stats.TotalGenerations, int64(10))
		// Some retries might have happened if codes collided
	})
}

func TestCollisionAwareGenerator_Concurrent(t *testing.T) {
	checker := newMockExistenceChecker()
	base := NewRandomGenerator(8)
	gen := NewCollisionAwareGenerator(base, checker, 10)

	numGoroutines := 50
	codesPerGoroutine := 50

	var wg sync.WaitGroup
	var mu sync.Mutex
	allCodes := make([]string, 0, numGoroutines*codesPerGoroutine)
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
					allCodes = append(allCodes, code)
					checker.Add(code) // Simulate storing in DB
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	assert.Empty(t, errors)
	assert.Len(t, allCodes, numGoroutines*codesPerGoroutine)

	// Verify all codes are unique
	seen := make(map[string]bool)
	for _, code := range allCodes {
		assert.False(t, seen[code], "duplicate code: %s", code)
		seen[code] = true
	}
}

func TestCollisionAwareGenerator_WithContext(t *testing.T) {
	t.Run("respects context cancellation", func(t *testing.T) {
		base := NewRandomGenerator(7)
		checker := &alwaysExistsChecker{}
		gen := NewCollisionAwareGenerator(base, checker, 1000) // High retry count

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		code, err := gen.GenerateWithContext(ctx)
		assert.Error(t, err)
		assert.Empty(t, code)
	})
}

func BenchmarkCollisionAwareGenerator(b *testing.B) {
	checker := &neverExistsChecker{}
	base := NewRandomGenerator(7)
	gen := NewCollisionAwareGenerator(base, checker, 3)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = gen.Generate()
	}
}
