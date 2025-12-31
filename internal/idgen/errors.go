package idgen

import "errors"

// Common errors for ID generation.
var (
	// ErrInvalidNodeID is returned when the node ID is out of valid range (0-1023).
	ErrInvalidNodeID = errors.New("node ID must be between 0 and 1023")

	// ErrClockMovedBackwards is returned when the system clock moves backwards.
	ErrClockMovedBackwards = errors.New("clock moved backwards, refusing to generate ID")

	// ErrMaxRetriesExceeded is returned when collision retry limit is reached.
	ErrMaxRetriesExceeded = errors.New("maximum retries exceeded for unique ID generation")
)
