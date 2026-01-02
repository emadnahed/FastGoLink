package analytics

import (
	"context"

	"github.com/emadnahed/FastGoLink/pkg/logger"
)

// ClickRepository defines the interface for persisting click counts.
type ClickRepository interface {
	BatchIncrementClickCounts(ctx context.Context, counts map[string]int64) error
}

// RepositoryFlusher implements Flusher using a repository.
type RepositoryFlusher struct {
	repo ClickRepository
	log  *logger.Logger
}

// NewRepositoryFlusher creates a new RepositoryFlusher.
func NewRepositoryFlusher(repo ClickRepository, log *logger.Logger) *RepositoryFlusher {
	return &RepositoryFlusher{
		repo: repo,
		log:  log,
	}
}

// FlushClicks persists click counts to the repository.
func (f *RepositoryFlusher) FlushClicks(ctx context.Context, counts map[string]int64) error {
	if len(counts) == 0 {
		return nil
	}

	err := f.repo.BatchIncrementClickCounts(ctx, counts)
	if err != nil {
		if f.log != nil {
			f.log.Error("failed to flush click counts", "error", err.Error(), "count", len(counts))
		}
		return err
	}

	if f.log != nil {
		total := int64(0)
		for _, c := range counts {
			total += c
		}
		f.log.Debug("flushed click counts", "urls", len(counts), "total_clicks", total)
	}

	return nil
}
