package idgen

import (
	"context"
	"sync/atomic"
)

// ExistenceChecker defines the interface for checking if a code exists.
type ExistenceChecker interface {
	// Exists returns true if the given code already exists in storage.
	Exists(ctx context.Context, code string) (bool, error)
}

// GeneratorStats holds statistics about code generation.
type GeneratorStats struct {
	TotalGenerations int64
	TotalRetries     int64
	TotalCollisions  int64
}

// CollisionAwareGenerator wraps a base generator and handles collisions.
type CollisionAwareGenerator struct {
	base       Generator
	checker    ExistenceChecker
	maxRetries int

	// Statistics
	totalGenerations atomic.Int64
	totalRetries     atomic.Int64
	totalCollisions  atomic.Int64
}

// NewCollisionAwareGenerator creates a new collision-aware generator.
// base: The underlying generator (Random or Snowflake)
// checker: Used to check if a code already exists
// maxRetries: Maximum number of retries on collision (0 means no retries)
func NewCollisionAwareGenerator(base Generator, checker ExistenceChecker, maxRetries int) *CollisionAwareGenerator {
	if maxRetries < 0 {
		maxRetries = 0
	}
	return &CollisionAwareGenerator{
		base:       base,
		checker:    checker,
		maxRetries: maxRetries,
	}
}

// Generate creates a unique short code, retrying on collisions.
// Uses a background context.
func (g *CollisionAwareGenerator) Generate() (string, error) {
	return g.GenerateWithContext(context.Background())
}

// GenerateWithContext creates a unique short code with context support.
// Respects context cancellation during retry loop.
func (g *CollisionAwareGenerator) GenerateWithContext(ctx context.Context) (string, error) {
	g.totalGenerations.Add(1)

	for attempt := 0; attempt <= g.maxRetries; attempt++ {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		// Generate a candidate code
		code, err := g.base.Generate()
		if err != nil {
			return "", err
		}

		// Check if it already exists
		exists, err := g.checker.Exists(ctx, code)
		if err != nil {
			return "", err
		}

		if !exists {
			// Found a unique code
			return code, nil
		}

		// Collision detected, will retry
		g.totalCollisions.Add(1)
		if attempt < g.maxRetries {
			g.totalRetries.Add(1)
		}
	}

	return "", ErrMaxRetriesExceeded
}

// Stats returns the current generation statistics.
func (g *CollisionAwareGenerator) Stats() GeneratorStats {
	return GeneratorStats{
		TotalGenerations: g.totalGenerations.Load(),
		TotalRetries:     g.totalRetries.Load(),
		TotalCollisions:  g.totalCollisions.Load(),
	}
}

// ResetStats resets all statistics to zero.
func (g *CollisionAwareGenerator) ResetStats() {
	g.totalGenerations.Store(0)
	g.totalRetries.Store(0)
	g.totalCollisions.Store(0)
}
