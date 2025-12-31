package database

import (
	"context"
	"fmt"
	"hash/fnv"
	"sort"
	"sync"

	"github.com/gourl/gourl/internal/config"
)

// ShardConfig represents configuration for a single shard.
type ShardConfig struct {
	ID     int
	Config *config.DatabaseConfig
}

// ShardRouter routes requests to appropriate database shards.
type ShardRouter struct {
	shards       []*Pool
	shardCount   int
	ring         []uint32
	ringToShard  map[uint32]int
	virtualNodes int
	mu           sync.RWMutex
}

// NewShardRouter creates a new shard router with the given shard configurations.
func NewShardRouter(ctx context.Context, configs []ShardConfig) (*ShardRouter, error) {
	if len(configs) == 0 {
		return nil, fmt.Errorf("at least one shard configuration is required")
	}

	router := &ShardRouter{
		shards:       make([]*Pool, len(configs)),
		shardCount:   len(configs),
		ringToShard:  make(map[uint32]int),
		virtualNodes: 150, // Virtual nodes per shard for better distribution
	}

	// Create pools for each shard
	for i, cfg := range configs {
		pool, err := NewPool(ctx, cfg.Config)
		if err != nil {
			// Close any already-created pools
			for j := 0; j < i; j++ {
				router.shards[j].Close()
			}
			return nil, fmt.Errorf("failed to create pool for shard %d: %w", cfg.ID, err)
		}
		router.shards[i] = pool
	}

	// Build consistent hash ring
	router.buildRing()

	return router, nil
}

// buildRing creates the consistent hash ring.
func (r *ShardRouter) buildRing() {
	r.ring = make([]uint32, 0, r.shardCount*r.virtualNodes)

	for shardIdx := 0; shardIdx < r.shardCount; shardIdx++ {
		for vn := 0; vn < r.virtualNodes; vn++ {
			key := fmt.Sprintf("shard-%d-vn-%d", shardIdx, vn)
			hash := hashKey(key)
			r.ring = append(r.ring, hash)
			r.ringToShard[hash] = shardIdx
		}
	}

	sort.Slice(r.ring, func(i, j int) bool {
		return r.ring[i] < r.ring[j]
	})
}

// GetShard returns the pool for the given key using consistent hashing.
func (r *ShardRouter) GetShard(key string) *Pool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.shardCount == 1 {
		return r.shards[0]
	}

	hash := hashKey(key)
	idx := r.findShardIndex(hash)
	shardIdx := r.ringToShard[r.ring[idx]]

	return r.shards[shardIdx]
}

// GetShardIndex returns the shard index for the given key.
func (r *ShardRouter) GetShardIndex(key string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.shardCount == 1 {
		return 0
	}

	hash := hashKey(key)
	idx := r.findShardIndex(hash)
	return r.ringToShard[r.ring[idx]]
}

// findShardIndex finds the index in the ring for the given hash.
func (r *ShardRouter) findShardIndex(hash uint32) int {
	idx := sort.Search(len(r.ring), func(i int) bool {
		return r.ring[i] >= hash
	})

	// Wrap around to the beginning if we're past the end
	if idx >= len(r.ring) {
		idx = 0
	}

	return idx
}

// GetAllShards returns all shard pools.
func (r *ShardRouter) GetAllShards() []*Pool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	shards := make([]*Pool, len(r.shards))
	copy(shards, r.shards)
	return shards
}

// ShardCount returns the number of shards.
func (r *ShardRouter) ShardCount() int {
	return r.shardCount
}

// HealthCheck checks the health of all shards.
func (r *ShardRouter) HealthCheck(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for i, shard := range r.shards {
		if err := shard.Ping(ctx); err != nil {
			return fmt.Errorf("shard %d health check failed: %w", i, err)
		}
	}
	return nil
}

// Close closes all shard connections.
func (r *ShardRouter) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, shard := range r.shards {
		shard.Close()
	}
}

// hashKey generates a hash for the given key.
func hashKey(key string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(key))
	return h.Sum32()
}

// SingleShardRouter creates a router with a single shard (no sharding).
func SingleShardRouter(ctx context.Context, cfg *config.DatabaseConfig) (*ShardRouter, error) {
	return NewShardRouter(ctx, []ShardConfig{
		{ID: 0, Config: cfg},
	})
}
