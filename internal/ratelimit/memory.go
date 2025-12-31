package ratelimit

import (
	"context"
	"sync"
	"time"
)

// MemoryLimiter implements an in-memory sliding window rate limiter.
type MemoryLimiter struct {
	config  Config
	entries sync.Map // map[string]*entry

	// For cleanup
	done chan struct{}
	wg   sync.WaitGroup
}

// entry holds the request timestamps for a single identifier.
type entry struct {
	mu         sync.Mutex
	timestamps []time.Time
}

// NewMemoryLimiter creates a new in-memory rate limiter.
func NewMemoryLimiter(cfg Config) *MemoryLimiter {
	m := &MemoryLimiter{
		config: cfg,
		done:   make(chan struct{}),
	}

	// Start cleanup goroutine
	m.wg.Add(1)
	go m.cleanupLoop()

	return m
}

// Allow checks if a request from the given identifier is allowed.
func (m *MemoryLimiter) Allow(ctx context.Context, identifier string) (*Result, error) {
	// Check context
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	now := time.Now()
	windowStart := now.Add(-m.config.Window)

	// Get or create entry
	entryVal, _ := m.entries.LoadOrStore(identifier, &entry{
		timestamps: make([]time.Time, 0, m.config.Requests),
	})
	e := entryVal.(*entry)

	e.mu.Lock()
	defer e.mu.Unlock()

	// Remove expired timestamps (sliding window)
	validTimestamps := make([]time.Time, 0, len(e.timestamps))
	for _, ts := range e.timestamps {
		if ts.After(windowStart) {
			validTimestamps = append(validTimestamps, ts)
		}
	}
	e.timestamps = validTimestamps

	count := len(e.timestamps)
	remaining := m.config.Requests - count

	// Calculate reset time (when the oldest entry will expire)
	var resetAfter time.Duration
	if len(e.timestamps) > 0 {
		oldestExpiry := e.timestamps[0].Add(m.config.Window)
		resetAfter = oldestExpiry.Sub(now)
		if resetAfter < 0 {
			resetAfter = 0
		}
	}

	// Check if allowed
	if count >= m.config.Requests {
		return &Result{
			Allowed:    false,
			Remaining:  0,
			ResetAfter: resetAfter,
			RetryAfter: resetAfter,
			Limit:      m.config.Requests,
		}, nil
	}

	// Add new timestamp
	e.timestamps = append(e.timestamps, now)

	return &Result{
		Allowed:    true,
		Remaining:  remaining - 1, // -1 because we just used one
		ResetAfter: resetAfter,
		RetryAfter: 0,
		Limit:      m.config.Requests,
	}, nil
}

// Reset clears the rate limit state for an identifier.
func (m *MemoryLimiter) Reset(ctx context.Context, identifier string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	m.entries.Delete(identifier)
	return nil
}

// Close releases resources held by the limiter.
func (m *MemoryLimiter) Close() error {
	close(m.done)
	m.wg.Wait()
	return nil
}

// cleanupLoop periodically removes expired entries.
func (m *MemoryLimiter) cleanupLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.Window)
	defer ticker.Stop()

	for {
		select {
		case <-m.done:
			return
		case <-ticker.C:
			m.cleanup()
		}
	}
}

// cleanup removes expired entries from the map.
func (m *MemoryLimiter) cleanup() {
	now := time.Now()
	windowStart := now.Add(-m.config.Window)

	m.entries.Range(func(key, value interface{}) bool {
		e := value.(*entry)
		e.mu.Lock()

		// Remove expired timestamps
		validTimestamps := make([]time.Time, 0, len(e.timestamps))
		for _, ts := range e.timestamps {
			if ts.After(windowStart) {
				validTimestamps = append(validTimestamps, ts)
			}
		}

		if len(validTimestamps) == 0 {
			// No valid timestamps, remove the entry
			e.mu.Unlock()
			m.entries.Delete(key)
		} else {
			e.timestamps = validTimestamps
			e.mu.Unlock()
		}

		return true
	})
}
