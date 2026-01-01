package analytics

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// mockFlusher is a mock implementation of the Flusher interface.
type mockFlusher struct {
	mu     sync.Mutex
	counts map[string]int64
	calls  int
}

func newMockFlusher() *mockFlusher {
	return &mockFlusher{
		counts: make(map[string]int64),
	}
}

func (m *mockFlusher) FlushClicks(ctx context.Context, counts map[string]int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	for code, count := range counts {
		m.counts[code] += count
	}
	return nil
}

func (m *mockFlusher) getCounts() map[string]int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make(map[string]int64)
	for k, v := range m.counts {
		result[k] = v
	}
	return result
}

func (m *mockFlusher) getCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func TestClickCounter_RecordClick(t *testing.T) {
	t.Run("records clicks for short code", func(t *testing.T) {
		flusher := newMockFlusher()
		counter := NewClickCounter(Config{
			FlushInterval: 50 * time.Millisecond,
			BatchSize:     100,
		}, flusher)
		defer counter.Stop()

		counter.RecordClick("abc123")
		counter.RecordClick("abc123")
		counter.RecordClick("xyz789")

		// Wait for flush
		time.Sleep(100 * time.Millisecond)

		counts := flusher.getCounts()
		assert.Equal(t, int64(2), counts["abc123"])
		assert.Equal(t, int64(1), counts["xyz789"])
	})

	t.Run("accumulates clicks between flushes", func(t *testing.T) {
		flusher := newMockFlusher()
		counter := NewClickCounter(Config{
			FlushInterval: 100 * time.Millisecond,
			BatchSize:     1000,
		}, flusher)
		defer counter.Stop()

		// Record many clicks
		for i := 0; i < 100; i++ {
			counter.RecordClick("abc123")
		}

		// Wait for flush
		time.Sleep(150 * time.Millisecond)

		counts := flusher.getCounts()
		assert.Equal(t, int64(100), counts["abc123"])
	})

	t.Run("flushes when batch size reached", func(t *testing.T) {
		flusher := newMockFlusher()
		counter := NewClickCounter(Config{
			FlushInterval: 10 * time.Second, // Long interval
			BatchSize:     10,               // Small batch size
		}, flusher)
		defer counter.Stop()

		// Record enough clicks to trigger batch flush
		for i := 0; i < 15; i++ {
			counter.RecordClick("abc123")
		}

		// Give time for batch flush
		time.Sleep(50 * time.Millisecond)

		counts := flusher.getCounts()
		assert.True(t, counts["abc123"] >= 10, "should have flushed at least batch size")
	})
}

func TestClickCounter_Stop(t *testing.T) {
	t.Run("flushes remaining clicks on stop", func(t *testing.T) {
		flusher := newMockFlusher()
		counter := NewClickCounter(Config{
			FlushInterval: 10 * time.Second, // Long interval
			BatchSize:     1000,             // Large batch
		}, flusher)

		counter.RecordClick("abc123")
		counter.RecordClick("abc123")
		counter.RecordClick("xyz789")

		// Stop should flush remaining
		counter.Stop()

		counts := flusher.getCounts()
		assert.Equal(t, int64(2), counts["abc123"])
		assert.Equal(t, int64(1), counts["xyz789"])
	})

	t.Run("is safe to call stop multiple times", func(t *testing.T) {
		flusher := newMockFlusher()
		counter := NewClickCounter(Config{
			FlushInterval: time.Second,
			BatchSize:     100,
		}, flusher)

		counter.RecordClick("abc123")

		// Should not panic
		counter.Stop()
		counter.Stop()
		counter.Stop()
	})

	t.Run("RecordClick after stop is ignored", func(t *testing.T) {
		flusher := newMockFlusher()
		counter := NewClickCounter(Config{
			FlushInterval: 10 * time.Second,
			BatchSize:     1000,
		}, flusher)

		counter.RecordClick("before-stop")
		counter.Stop()

		// These should be ignored
		counter.RecordClick("after-stop")
		counter.RecordClick("after-stop")

		counts := flusher.getCounts()
		assert.Equal(t, int64(1), counts["before-stop"])
		assert.Equal(t, int64(0), counts["after-stop"])
	})
}

func TestClickCounter_Concurrency(t *testing.T) {
	t.Run("handles concurrent clicks safely", func(t *testing.T) {
		flusher := newMockFlusher()
		counter := NewClickCounter(Config{
			FlushInterval: 50 * time.Millisecond,
			BatchSize:     1000,
		}, flusher)
		defer counter.Stop()

		var wg sync.WaitGroup
		clicksPerGoroutine := 100
		numGoroutines := 10

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < clicksPerGoroutine; j++ {
					counter.RecordClick("concurrent-code")
				}
			}()
		}

		wg.Wait()

		// Wait for flush
		time.Sleep(100 * time.Millisecond)

		counts := flusher.getCounts()
		expectedTotal := int64(numGoroutines * clicksPerGoroutine)
		assert.Equal(t, expectedTotal, counts["concurrent-code"])
	})
}

func TestClickCounter_NonBlocking(t *testing.T) {
	t.Run("RecordClick does not block", func(t *testing.T) {
		flusher := newMockFlusher()
		counter := NewClickCounter(Config{
			FlushInterval: time.Second,
			BatchSize:     100,
		}, flusher)
		defer counter.Stop()

		// Should complete very quickly
		start := time.Now()
		for i := 0; i < 1000; i++ {
			counter.RecordClick("fast-code")
		}
		elapsed := time.Since(start)

		// Should be extremely fast (< 10ms for 1000 calls)
		assert.True(t, elapsed < 10*time.Millisecond, "RecordClick should be non-blocking, took %v", elapsed)
	})
}

func TestClickCounter_GetStats(t *testing.T) {
	t.Run("returns in-memory stats", func(t *testing.T) {
		flusher := newMockFlusher()
		counter := NewClickCounter(Config{
			FlushInterval: 10 * time.Second,
			BatchSize:     1000,
		}, flusher)
		defer counter.Stop()

		counter.RecordClick("abc123")
		counter.RecordClick("abc123")
		counter.RecordClick("xyz789")

		// Allow time for async processing
		time.Sleep(10 * time.Millisecond)

		stats := counter.GetPendingStats()
		assert.Equal(t, int64(2), stats["abc123"])
		assert.Equal(t, int64(1), stats["xyz789"])
	})
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, 10*time.Second, cfg.FlushInterval)
	assert.Equal(t, 100, cfg.BatchSize)
	assert.Equal(t, 10000, cfg.ChannelBuffer)
}

// benchmarkCounter benchmarks click recording
func BenchmarkClickCounter_RecordClick(b *testing.B) {
	flusher := newMockFlusher()
	counter := NewClickCounter(Config{
		FlushInterval: time.Minute,
		BatchSize:     10000,
		ChannelBuffer: 10000,
	}, flusher)
	defer counter.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		counter.RecordClick("bench-code")
	}
}

func BenchmarkClickCounter_RecordClick_Parallel(b *testing.B) {
	flusher := newMockFlusher()
	counter := NewClickCounter(Config{
		FlushInterval: time.Minute,
		BatchSize:     10000,
		ChannelBuffer: 10000,
	}, flusher)
	defer counter.Stop()

	var counter64 int64

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			n := atomic.AddInt64(&counter64, 1)
			counter.RecordClick("bench-code-" + string(rune(n%26+'a')))
		}
	})
}
