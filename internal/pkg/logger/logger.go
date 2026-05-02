package logger

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"time"
)

// Level represents a log severity.
type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
	FATAL
)

func (l Level) String() string {
	switch l {
	case DEBUG: return "DEBUG"
	case INFO:  return "INFO"
	case WARN:  return "WARN"
	case ERROR: return "ERROR"
	case FATAL: return "FATAL"
	default:    return "UNKNOWN"
	}
}

// jsonWriter intercepts log output and formats it as JSON.
type jsonWriter struct {
	service string
}

func (w *jsonWriter) Write(p []byte) (int, error) {
	msg := strings.TrimSpace(string(p))
	// Strip the standard log prefix: "2006/01/02 15:04:05.123456 file.go:123: "
	// The prefix format is: date time.microseconds file:line: message
	if idx := strings.Index(msg, ": "); idx != -1 {
		if idx2 := strings.Index(msg[idx+2:], ": "); idx2 != -1 {
			msg = msg[idx+2+idx2+2:]
		}
	}
	entry := map[string]any{
		"ts":      time.Now().UTC().Format(time.RFC3339Nano),
		"level":   "INFO",
		"service": w.service,
		"msg":     msg,
	}
	b, _ := json.Marshal(entry)
	fmt.Fprintln(os.Stdout, string(b))
	return len(p), nil
}

// New returns a *log.Logger that outputs JSON structured logs.
func New(serviceName string) *log.Logger {
	return log.New(&jsonWriter{service: serviceName}, "", log.LstdFlags|log.Lmicroseconds|log.Lshortfile)
}

// Structured wraps a *log.Logger and adds leveled methods.
type Structured struct {
	*log.Logger
	service string
}

// NewStructured returns a Structured logger with additional leveled methods.
func NewStructured(serviceName string) *Structured {
	return &Structured{
		Logger:  New(serviceName),
		service: serviceName,
	}
}

func (s *Structured) logf(level Level, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	entry := map[string]any{
		"ts":      time.Now().UTC().Format(time.RFC3339Nano),
		"level":   level.String(),
		"service": s.service,
		"msg":     msg,
	}
	if level >= ERROR {
		_, file, line, ok := runtime.Caller(2)
		if ok {
			entry["caller"] = fmt.Sprintf("%s:%d", file, line)
		}
	}
	b, _ := json.Marshal(entry)
	fmt.Fprintln(os.Stdout, string(b))
	if level == FATAL {
		os.Exit(1)
	}
}

func (s *Structured) Debug(format string, args ...any) { s.logf(DEBUG, format, args...) }
func (s *Structured) Info(format string, args ...any)  { s.logf(INFO, format, args...) }
func (s *Structured) Warn(format string, args ...any)  { s.logf(WARN, format, args...) }
func (s *Structured) Error(format string, args ...any) { s.logf(ERROR, format, args...) }
func (s *Structured) Fatal(format string, args ...any) { s.logf(FATAL, format, args...) }
