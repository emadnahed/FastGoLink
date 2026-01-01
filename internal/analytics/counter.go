// Package analytics provides click tracking and analytics functionality.
package analytics

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// Flusher defines the interface for persisting click counts.
type Flusher interface {
	FlushClicks(ctx context.Context, counts map[string]int64) error
}

// Config holds configuration for the ClickCounter.
type Config struct {
	FlushInterval time.Duration // How often to flush accumulated counts
	BatchSize     int           // Flush when this many unique codes accumulated
	ChannelBuffer int           // Size of the click channel buffer
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		FlushInterval: 10 * time.Second,
		BatchSize:     100,
		ChannelBuffer: 10000,
	}
}

// ClickCounter provides non-blocking, batched click counting.
type ClickCounter struct {
	flusher Flusher
	cfg     Config

	clickChan    chan string
	counts       map[string]int64
	countsMu     sync.Mutex
	pendingCount int64 // total pending clicks (for batch size check)

	stopOnce sync.Once
	stopChan chan struct{}
	doneChan chan struct{}
	stopped  atomic.Bool
}

// NewClickCounter creates a new ClickCounter instance.
func NewClickCounter(cfg Config, flusher Flusher) *ClickCounter {
	if cfg.ChannelBuffer <= 0 {
		cfg.ChannelBuffer = DefaultConfig().ChannelBuffer
	}

	c := &ClickCounter{
		flusher:   flusher,
		cfg:       cfg,
		clickChan: make(chan string, cfg.ChannelBuffer),
		counts:    make(map[string]int64),
		stopChan:  make(chan struct{}),
		doneChan:  make(chan struct{}),
	}

	go c.run()
	return c
}

// RecordClick records a click for a short code (non-blocking).
func (c *ClickCounter) RecordClick(shortCode string) {
	if c.stopped.Load() {
		return
	}

	// Non-blocking send - drop if buffer is full
	select {
	case c.clickChan <- shortCode:
	default:
		// Channel full, click dropped (acceptable for analytics)
	}
}

// Stop stops the click counter and flushes remaining counts.
func (c *ClickCounter) Stop() {
	c.stopOnce.Do(func() {
		c.stopped.Store(true)
		close(c.stopChan)
		<-c.doneChan
	})
}

// GetPendingStats returns a snapshot of pending (unflushed) click counts.
func (c *ClickCounter) GetPendingStats() map[string]int64 {
	c.countsMu.Lock()
	defer c.countsMu.Unlock()

	result := make(map[string]int64, len(c.counts))
	for k, v := range c.counts {
		result[k] = v
	}
	return result
}

// run is the main loop that processes clicks and flushes periodically.
func (c *ClickCounter) run() {
	defer close(c.doneChan)

	ticker := time.NewTicker(c.cfg.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case shortCode := <-c.clickChan:
			c.countsMu.Lock()
			c.counts[shortCode]++
			c.pendingCount++
			shouldFlush := int(c.pendingCount) >= c.cfg.BatchSize
			c.countsMu.Unlock()

			if shouldFlush {
				c.flush()
			}

		case <-ticker.C:
			c.flush()

		case <-c.stopChan:
			// Drain remaining clicks from channel
			c.drainChannel()
			// Final flush
			c.flush()
			return
		}
	}
}

// drainChannel processes any remaining clicks in the channel.
func (c *ClickCounter) drainChannel() {
	for {
		select {
		case shortCode := <-c.clickChan:
			c.countsMu.Lock()
			c.counts[shortCode]++
			c.pendingCount++
			c.countsMu.Unlock()
		default:
			return
		}
	}
}

// flush sends accumulated counts to the flusher and resets.
func (c *ClickCounter) flush() {
	c.countsMu.Lock()
	if len(c.counts) == 0 {
		c.countsMu.Unlock()
		return
	}

	// Swap maps for minimal lock time
	toFlush := c.counts
	c.counts = make(map[string]int64)
	c.pendingCount = 0
	c.countsMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Fire and forget - errors are logged but don't block
	_ = c.flusher.FlushClicks(ctx, toFlush)
}
