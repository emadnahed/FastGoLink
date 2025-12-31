// Package logger provides structured logging utilities.
package logger

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// Level represents logging severity.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// String returns the string representation of the level.
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "INFO"
	}
}

// ParseLevel parses a string into a Level.
func ParseLevel(s string) Level {
	switch strings.ToLower(s) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

// Logger is a structured JSON logger.
type Logger struct {
	output io.Writer
	level  Level
	fields map[string]interface{}
	mu     sync.Mutex
}

// New creates a new Logger with the specified output and level.
func New(output io.Writer, level string) *Logger {
	if output == nil {
		output = os.Stdout
	}
	return &Logger{
		output: output,
		level:  ParseLevel(level),
		fields: make(map[string]interface{}),
	}
}

// With returns a new Logger with additional fields.
func (l *Logger) With(keyvals ...interface{}) *Logger {
	newLogger := &Logger{
		output: l.output,
		level:  l.level,
		fields: make(map[string]interface{}),
	}

	// Copy existing fields
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}

	// Add new fields
	for i := 0; i < len(keyvals)-1; i += 2 {
		if key, ok := keyvals[i].(string); ok {
			newLogger.fields[key] = keyvals[i+1]
		}
	}

	return newLogger
}

// Debug logs a message at debug level.
func (l *Logger) Debug(msg string, keyvals ...interface{}) {
	l.log(LevelDebug, msg, keyvals...)
}

// Info logs a message at info level.
func (l *Logger) Info(msg string, keyvals ...interface{}) {
	l.log(LevelInfo, msg, keyvals...)
}

// Warn logs a message at warn level.
func (l *Logger) Warn(msg string, keyvals ...interface{}) {
	l.log(LevelWarn, msg, keyvals...)
}

// Error logs a message at error level.
func (l *Logger) Error(msg string, keyvals ...interface{}) {
	l.log(LevelError, msg, keyvals...)
}

// log writes a log entry if the level is enabled.
func (l *Logger) log(level Level, msg string, keyvals ...interface{}) {
	if level < l.level {
		return
	}

	entry := make(map[string]interface{})

	// Add persistent fields
	for k, v := range l.fields {
		entry[k] = v
	}

	// Add standard fields
	entry["time"] = time.Now().UTC().Format(time.RFC3339)
	entry["level"] = level.String()
	entry["msg"] = msg

	// Add dynamic keyvals
	for i := 0; i < len(keyvals)-1; i += 2 {
		if key, ok := keyvals[i].(string); ok {
			entry[key] = keyvals[i+1]
		}
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	data, err := json.Marshal(entry)
	if err != nil {
		return
	}

	_, _ = l.output.Write(data)
	_, _ = l.output.Write([]byte("\n"))
}
