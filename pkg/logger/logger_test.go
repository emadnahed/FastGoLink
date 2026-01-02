package logger

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, "debug")

	assert.NotNil(t, log)
}

func TestLogger_Info(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, "info")

	log.Info("test message", "key", "value")

	var entry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "INFO", entry["level"])
	assert.Equal(t, "test message", entry["msg"])
	assert.Equal(t, "value", entry["key"])
	assert.NotEmpty(t, entry["time"])
}

func TestLogger_Error(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, "error")

	log.Error("error occurred", "error", "something went wrong")

	var entry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "ERROR", entry["level"])
	assert.Equal(t, "error occurred", entry["msg"])
	assert.Equal(t, "something went wrong", entry["error"])
}

func TestLogger_Debug(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, "debug")

	log.Debug("debug message", "details", "debugging info")

	var entry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "DEBUG", entry["level"])
	assert.Equal(t, "debug message", entry["msg"])
}

func TestLogger_Warn(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, "warn")

	log.Warn("warning message", "warning", "be careful")

	var entry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "WARN", entry["level"])
	assert.Equal(t, "warning message", entry["msg"])
}

func TestLogger_LevelFiltering(t *testing.T) {
	tests := []struct {
		name      string
		level     string
		logFunc   func(*Logger)
		shouldLog bool
	}{
		{"debug logs at debug level", "debug", func(l *Logger) { l.Debug("msg") }, true},
		{"info logs at debug level", "debug", func(l *Logger) { l.Info("msg") }, true},
		{"debug skipped at info level", "info", func(l *Logger) { l.Debug("msg") }, false},
		{"info logs at info level", "info", func(l *Logger) { l.Info("msg") }, true},
		{"warn logs at info level", "info", func(l *Logger) { l.Warn("msg") }, true},
		{"info skipped at warn level", "warn", func(l *Logger) { l.Info("msg") }, false},
		{"error logs at error level", "error", func(l *Logger) { l.Error("msg") }, true},
		{"warn skipped at error level", "error", func(l *Logger) { l.Warn("msg") }, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			log := New(&buf, tt.level)
			tt.logFunc(log)

			if tt.shouldLog {
				assert.NotEmpty(t, buf.String(), "expected log output")
			} else {
				assert.Empty(t, buf.String(), "expected no log output")
			}
		})
	}
}

func TestLogger_With(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, "info")

	childLog := log.With("service", "fastgolink", "version", "1.0")
	childLog.Info("request handled")

	var entry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "fastgolink", entry["service"])
	assert.Equal(t, "1.0", entry["version"])
}

func TestLogger_JSONOutput(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, "info")

	log.Info("json test", "nested", map[string]string{"foo": "bar"})

	output := buf.String()
	assert.True(t, strings.HasPrefix(output, "{"))
	assert.True(t, strings.HasSuffix(strings.TrimSpace(output), "}"))
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected Level
	}{
		{"debug", LevelDebug},
		{"DEBUG", LevelDebug},
		{"info", LevelInfo},
		{"INFO", LevelInfo},
		{"warn", LevelWarn},
		{"warning", LevelWarn},
		{"error", LevelError},
		{"ERROR", LevelError},
		{"invalid", LevelInfo}, // default to info
		{"", LevelInfo},        // default to info
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, ParseLevel(tt.input))
		})
	}
}

func TestLevel_String(t *testing.T) {
	tests := []struct {
		level    Level
		expected string
	}{
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
		{Level(999), "INFO"}, // invalid level defaults to INFO
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.level.String())
	}
}

func TestNew_NilOutput(t *testing.T) {
	// When nil output is provided, it should default to os.Stdout
	log := New(nil, "info")
	assert.NotNil(t, log)
	// We can't easily test os.Stdout was used, but the logger should work
}

func TestLogger_With_NonStringKey(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, "info")

	// Pass non-string keys - they should be skipped
	childLog := log.With(123, "value", "validkey", "validvalue")
	childLog.Info("test")

	var entry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	// Non-string key (123) should be skipped
	_, hasIntKey := entry["123"]
	assert.False(t, hasIntKey)

	// Valid string key should be present
	assert.Equal(t, "validvalue", entry["validkey"])
}

func TestLogger_With_CopiesExistingFields(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, "info")

	// Create first child with some fields
	child1 := log.With("service", "fastgolink")

	// Create second child from first child - should copy service field
	child2 := child1.With("request_id", "abc123")
	child2.Info("test")

	var entry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	// Both fields should be present
	assert.Equal(t, "fastgolink", entry["service"])
	assert.Equal(t, "abc123", entry["request_id"])
}

func TestLogger_Log_NonStringKeyval(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, "info")

	// Pass non-string key in log call - should be skipped
	log.Info("message", 42, "skipme", "good", "value")

	var entry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "value", entry["good"])
}

func TestLogger_Log_MarshalError(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, "info")

	// Channels can't be marshalled to JSON
	ch := make(chan int)
	log.Info("message", "channel", ch)

	// Output should be empty because marshal failed
	assert.Empty(t, buf.String())
}
