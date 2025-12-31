// Package testutil provides shared utilities for tests.
package testutil

import (
	"os"
	"testing"
)

// SetEnv sets an environment variable for the duration of a test.
func SetEnv(t *testing.T, key, value string) {
	t.Helper()
	old := os.Getenv(key)
	if err := os.Setenv(key, value); err != nil {
		t.Fatalf("failed to set env %s: %v", key, err)
	}
	t.Cleanup(func() {
		if old == "" {
			os.Unsetenv(key)
		} else {
			os.Setenv(key, old)
		}
	})
}

// SkipIfShort skips long-running tests when -short flag is used.
func SkipIfShort(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}
}

// SkipIfNoDocker skips tests that require Docker.
func SkipIfNoDocker(t *testing.T) {
	t.Helper()
	if os.Getenv("DOCKER_AVAILABLE") != "true" {
		t.Skip("skipping test: Docker not available")
	}
}
