package database

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gourl/gourl/internal/config"
)

func TestHashKey(t *testing.T) {
	tests := []struct {
		key1, key2 string
		sameHash   bool
	}{
		{"abc", "abc", true},
		{"abc", "def", false},
		{"short1", "short1", true},
		{"short1", "short2", false},
	}

	for _, tt := range tests {
		h1 := hashKey(tt.key1)
		h2 := hashKey(tt.key2)

		if tt.sameHash {
			assert.Equal(t, h1, h2, "expected same hash for %q and %q", tt.key1, tt.key2)
		} else {
			assert.NotEqual(t, h1, h2, "expected different hash for %q and %q", tt.key1, tt.key2)
		}
	}
}

func TestHashKey_Consistency(t *testing.T) {
	// Same key should always produce same hash
	key := "test-key-12345"

	hash1 := hashKey(key)
	hash2 := hashKey(key)
	hash3 := hashKey(key)

	assert.Equal(t, hash1, hash2)
	assert.Equal(t, hash2, hash3)
}

func TestHashKey_Distribution(t *testing.T) {
	// Test that hashes are reasonably distributed
	hashes := make(map[uint32]int)

	for i := 0; i < 10000; i++ {
		key := "key-" + string(rune(i))
		hash := hashKey(key)
		hashes[hash]++
	}

	// Should have many unique hashes (low collision)
	assert.Greater(t, len(hashes), 9000, "hash function should have good distribution")
}

func TestSingleShardRouter(t *testing.T) {
	skipIfNoPostgres(t)

	ctx := context.Background()
	cfg := testDBConfig()

	router, err := SingleShardRouter(ctx, cfg)
	require.NoError(t, err)
	defer router.Close()

	assert.Equal(t, 1, router.ShardCount())

	// All keys should go to the same shard
	shard1 := router.GetShard("key1")
	shard2 := router.GetShard("key2")
	shard3 := router.GetShard("completely-different-key")

	assert.Same(t, shard1, shard2)
	assert.Same(t, shard2, shard3)
}

func TestShardRouter_GetShardIndex(t *testing.T) {
	skipIfNoPostgres(t)

	ctx := context.Background()
	cfg := testDBConfig()

	router, err := SingleShardRouter(ctx, cfg)
	require.NoError(t, err)
	defer router.Close()

	// With single shard, all should return 0
	assert.Equal(t, 0, router.GetShardIndex("key1"))
	assert.Equal(t, 0, router.GetShardIndex("key2"))
}

func TestShardRouter_Deterministic(t *testing.T) {
	skipIfNoPostgres(t)

	ctx := context.Background()
	cfg := testDBConfig()

	router, err := SingleShardRouter(ctx, cfg)
	require.NoError(t, err)
	defer router.Close()

	key := "test-short-code"

	// Same key should always route to same shard
	idx1 := router.GetShardIndex(key)
	idx2 := router.GetShardIndex(key)
	idx3 := router.GetShardIndex(key)

	assert.Equal(t, idx1, idx2)
	assert.Equal(t, idx2, idx3)
}

func TestShardRouter_HealthCheck(t *testing.T) {
	skipIfNoPostgres(t)

	ctx := context.Background()
	cfg := testDBConfig()

	router, err := SingleShardRouter(ctx, cfg)
	require.NoError(t, err)
	defer router.Close()

	err = router.HealthCheck(ctx)
	assert.NoError(t, err)
}

func TestShardRouter_GetAllShards(t *testing.T) {
	skipIfNoPostgres(t)

	ctx := context.Background()
	cfg := testDBConfig()

	router, err := SingleShardRouter(ctx, cfg)
	require.NoError(t, err)
	defer router.Close()

	shards := router.GetAllShards()
	assert.Len(t, shards, 1)
}

func TestShardRouter_Close(t *testing.T) {
	skipIfNoPostgres(t)

	ctx := context.Background()
	cfg := testDBConfig()

	router, err := SingleShardRouter(ctx, cfg)
	require.NoError(t, err)

	// Get a shard
	shard := router.GetShard("test")
	require.NotNil(t, shard)

	// Close router
	router.Close()

	// After close, ping should fail
	err = shard.Ping(ctx)
	assert.Error(t, err)
}

func TestNewShardRouter_NoConfigs(t *testing.T) {
	ctx := context.Background()

	_, err := NewShardRouter(ctx, []ShardConfig{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least one shard")
}

func TestNewShardRouter_InvalidConfig(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	configs := []ShardConfig{
		{
			ID: 0,
			Config: &config.DatabaseConfig{
				Host:     "invalid-host",
				Port:     5432,
				User:     "invalid",
				Password: "invalid",
				DBName:   "invalid",
				SSLMode:  "disable",
			},
		},
	}

	_, err := NewShardRouter(ctx, configs)
	assert.Error(t, err)
}

func TestShardRouter_MultipleShards(t *testing.T) {
	skipIfNoPostgres(t)

	ctx := context.Background()
	cfg := testDBConfig()

	// Create router with 2 "virtual" shards pointing to same DB
	// In production, these would be different databases
	configs := []ShardConfig{
		{ID: 0, Config: cfg},
		{ID: 1, Config: cfg},
	}

	router, err := NewShardRouter(ctx, configs)
	require.NoError(t, err)
	defer router.Close()

	assert.Equal(t, 2, router.ShardCount())

	// Track which shard each key goes to
	shardCounts := make(map[int]int)
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key-%d", i)
		idx := router.GetShardIndex(key)
		shardCounts[idx]++
	}

	// Both shards should get some keys (at least 10% each for non-flaky test)
	assert.Greater(t, shardCounts[0], 100, "shard 0 should get significant portion")
	assert.Greater(t, shardCounts[1], 100, "shard 1 should get significant portion")
}

func TestShardRouter_ConsistentHashing(t *testing.T) {
	skipIfNoPostgres(t)

	ctx := context.Background()
	cfg := testDBConfig()

	// Create router with 3 shards
	configs := []ShardConfig{
		{ID: 0, Config: cfg},
		{ID: 1, Config: cfg},
		{ID: 2, Config: cfg},
	}

	router, err := NewShardRouter(ctx, configs)
	require.NoError(t, err)
	defer router.Close()

	// Record which shard each key goes to
	keyToShard := make(map[string]int)
	keys := make([]string, 100)
	for i := 0; i < 100; i++ {
		key := "shortcode-" + string(rune(i))
		keys[i] = key
		keyToShard[key] = router.GetShardIndex(key)
	}

	// Verify consistency - same keys should always go to same shards
	for _, key := range keys {
		idx := router.GetShardIndex(key)
		assert.Equal(t, keyToShard[key], idx, "key %s should consistently route to same shard", key)
	}
}
