package idgen

import (
	"sync"
	"time"
)

// Snowflake epoch: January 1, 2024 00:00:00 UTC
// Using a custom epoch allows for 69 years of IDs from this date.
const snowflakeEpoch int64 = 1704067200000 // milliseconds

// Bit allocation for Snowflake IDs:
// - 41 bits for timestamp (milliseconds since epoch) - ~69 years
// - 10 bits for node ID (0-1023)
// - 12 bits for sequence number (0-4095 per millisecond)
const (
	nodeBits     = 10
	sequenceBits = 12

	maxNodeID   = (1 << nodeBits) - 1     // 1023
	maxSequence = (1 << sequenceBits) - 1 // 4095

	nodeShift      = sequenceBits
	timestampShift = nodeBits + sequenceBits
)

// SnowflakeGenerator generates unique, time-ordered IDs using Snowflake algorithm.
// IDs are encoded as Base62 strings for URL-safe short codes.
type SnowflakeGenerator struct {
	mu        sync.Mutex
	nodeID    int64
	sequence  int64
	lastTime  int64
	minLength int
}

// NewSnowflakeGenerator creates a new SnowflakeGenerator with the given node ID.
// nodeID must be between 0 and 1023 (inclusive).
// minLength specifies the minimum length of the generated Base62 code.
func NewSnowflakeGenerator(nodeID int64, minLength int) (*SnowflakeGenerator, error) {
	if nodeID < 0 || nodeID > maxNodeID {
		return nil, ErrInvalidNodeID
	}
	if minLength < 1 {
		minLength = DefaultCodeLength
	}
	return &SnowflakeGenerator{
		nodeID:    nodeID,
		minLength: minLength,
	}, nil
}

// Generate creates a new unique, time-ordered short code.
// Thread-safe and monotonically increasing within the same node.
func (g *SnowflakeGenerator) Generate() (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now().UnixMilli()

	if now == g.lastTime {
		g.sequence = (g.sequence + 1) & maxSequence
		if g.sequence == 0 {
			// Sequence overflow, wait for next millisecond
			for now <= g.lastTime {
				now = time.Now().UnixMilli()
			}
		}
	} else if now < g.lastTime {
		// Clock moved backwards
		return "", ErrClockMovedBackwards
	} else {
		g.sequence = 0
	}

	g.lastTime = now

	// Generate Snowflake ID
	id := ((now - snowflakeEpoch) << timestampShift) |
		(g.nodeID << nodeShift) |
		g.sequence

	// #nosec G115 -- id is always positive due to snowflake ID construction
	return EncodeWithPadding(uint64(id), g.minLength), nil
}

// NodeID returns the configured node ID.
func (g *SnowflakeGenerator) NodeID() int64 {
	return g.nodeID
}
