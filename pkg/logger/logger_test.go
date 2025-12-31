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

	childLog := log.With("service", "gourl", "version", "1.0")
	childLog.Info("request handled")

	var entry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "gourl", entry["service"])
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
