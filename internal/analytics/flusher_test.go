package analytics

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gourl/gourl/pkg/logger"
)

// mockClickRepository implements ClickRepository for testing.
type mockClickRepository struct {
	batchIncrementCalled bool
	batchCounts          map[string]int64
	batchErr             error
}

func (m *mockClickRepository) BatchIncrementClickCounts(ctx context.Context, counts map[string]int64) error {
	m.batchIncrementCalled = true
	m.batchCounts = counts
	return m.batchErr
}

func TestNewRepositoryFlusher(t *testing.T) {
	repo := &mockClickRepository{}
	log := logger.New(os.Stdout, "debug")

	flusher := NewRepositoryFlusher(repo, log)

	require.NotNil(t, flusher)
	assert.NotNil(t, flusher.repo)
	assert.NotNil(t, flusher.log)
}

func TestRepositoryFlusher_FlushClicks(t *testing.T) {
	t.Run("flushes click counts to repository", func(t *testing.T) {
		repo := &mockClickRepository{}
		log := logger.New(os.Stdout, "debug")
		flusher := NewRepositoryFlusher(repo, log)

		counts := map[string]int64{
			"abc123": 5,
			"def456": 3,
		}

		err := flusher.FlushClicks(context.Background(), counts)

		require.NoError(t, err)
		assert.True(t, repo.batchIncrementCalled)
		assert.Equal(t, counts, repo.batchCounts)
	})

	t.Run("returns nil for empty counts", func(t *testing.T) {
		repo := &mockClickRepository{}
		flusher := NewRepositoryFlusher(repo, nil)

		err := flusher.FlushClicks(context.Background(), map[string]int64{})

		require.NoError(t, err)
		assert.False(t, repo.batchIncrementCalled)
	})

	t.Run("returns nil for nil counts", func(t *testing.T) {
		repo := &mockClickRepository{}
		flusher := NewRepositoryFlusher(repo, nil)

		err := flusher.FlushClicks(context.Background(), nil)

		require.NoError(t, err)
		assert.False(t, repo.batchIncrementCalled)
	})

	t.Run("returns error from repository", func(t *testing.T) {
		repo := &mockClickRepository{
			batchErr: errors.New("database error"),
		}
		log := logger.New(os.Stdout, "error")
		flusher := NewRepositoryFlusher(repo, log)

		counts := map[string]int64{"abc123": 5}
		err := flusher.FlushClicks(context.Background(), counts)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "database error")
	})

	t.Run("works without logger", func(t *testing.T) {
		repo := &mockClickRepository{}
		flusher := NewRepositoryFlusher(repo, nil)

		counts := map[string]int64{"abc123": 5}
		err := flusher.FlushClicks(context.Background(), counts)

		require.NoError(t, err)
		assert.True(t, repo.batchIncrementCalled)
	})

	t.Run("logs error without logger (no panic)", func(t *testing.T) {
		repo := &mockClickRepository{
			batchErr: errors.New("database error"),
		}
		flusher := NewRepositoryFlusher(repo, nil)

		counts := map[string]int64{"abc123": 5}
		err := flusher.FlushClicks(context.Background(), counts)

		require.Error(t, err)
	})
}
