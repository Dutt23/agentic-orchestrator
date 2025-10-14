package logger

import (
	"context"
	"log/slog"
	"os"
	"runtime/debug"
	"time"

	"github.com/lmittmann/tint"
)

// Logger wraps slog.Logger with contextual fields
type Logger struct {
	*slog.Logger
}

// New creates a new logger
func New(level, format string) *Logger {
	var handler slog.Handler

	logLevel := parseLevel(level)

	switch format {
	case "json":
		opts := &slog.HandlerOptions{
			Level: logLevel,
		}
		handler = slog.NewJSONHandler(os.Stdout, opts)
	default:
		// Use tint for beautiful colored console output
		handler = tint.NewHandler(os.Stdout, &tint.Options{
			Level:      logLevel,
			TimeFormat: time.TimeOnly, // HH:MM:SS
			AddSource:  false,          // Don't show source file by default
		})
	}

	return &Logger{
		Logger: slog.New(handler),
	}
}

// WithContext returns a logger with trace_id from context
func (l *Logger) WithContext(ctx context.Context) *Logger {
	if traceID := ctx.Value("trace_id"); traceID != nil {
		return &Logger{
			Logger: l.With("trace_id", traceID),
		}
	}
	return l
}

// WithFields returns a logger with additional fields
func (l *Logger) WithFields(fields map[string]any) *Logger {
	args := make([]any, 0, len(fields)*2)
	for k, v := range fields {
		args = append(args, k, v)
	}
	return &Logger{
		Logger: l.With(args...),
	}
}

// WithRunID adds run_id to logger context
func (l *Logger) WithRunID(runID string) *Logger {
	return &Logger{
		Logger: l.With("run_id", runID),
	}
}

// WithNodeID adds node_id to logger context
func (l *Logger) WithNodeID(nodeID string) *Logger {
	return &Logger{
		Logger: l.With("node_id", nodeID),
	}
}

// Error logs an error with stack trace
func (l *Logger) Error(msg string, args ...any) {
	// Add stack trace to args
	stack := string(debug.Stack())
	args = append(args, "stack", stack)
	l.Logger.Error(msg, args...)
}

// ErrorContext logs an error with context and stack trace
func (l *Logger) ErrorContext(ctx context.Context, msg string, args ...any) {
	// Add stack trace to args
	stack := string(debug.Stack())
	args = append(args, "stack", stack)
	l.Logger.ErrorContext(ctx, msg, args...)
}

func parseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}