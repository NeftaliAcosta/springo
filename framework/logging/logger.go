package logging

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// TraceIDKey must match the one defined in framework/web/tracing.go
var (
	TraceIDKey   = "springo_trace_id"
	SpanIDKey    = "springo_span_id"
	currentLevel = &slog.LevelVar{}
)

// ContextHandler is a custom slog handler that automatically adds trace_id and span_id from context
type ContextHandler struct {
	slog.Handler
}

func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if ctx != nil {
		if traceID, ok := ctx.Value(TraceIDKey).(string); ok {
			r.AddAttrs(slog.String("trace_id", traceID))
		}
		if spanID, ok := ctx.Value(SpanIDKey).(string); ok {
			r.AddAttrs(slog.String("span_id", spanID))
		}
	}
	return h.Handler.Handle(ctx, r)
}

// Initialize setups the global slog logger based on properties
func Initialize(props *LoggingProperties) {
	if props == nil {
		props = &LoggingProperties{Level: "INFO", Format: "text"}
	}

	var handler slog.Handler
	level := props.GetSlogLevel()
	currentLevel.Set(level)

	opts := &slog.HandlerOptions{
		Level: currentLevel,
	}

	if strings.ToLower(props.Format) == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	// Wrap with our ContextHandler to support tracing
	logger := slog.New(&ContextHandler{Handler: handler})
	slog.SetDefault(logger)
}

// SetLevel changes the active logging level dynamically
func SetLevel(levelStr string) {
	var lvl slog.Level
	switch strings.ToUpper(levelStr) {
	case "DEBUG":
		lvl = slog.LevelDebug
	case "WARN":
		lvl = slog.LevelWarn
	case "ERROR":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	currentLevel.Set(lvl)
}

// GetLevel returns the string representation of the active logging level
func GetLevel() string {
	return currentLevel.Level().String()
}

// Global Logger Accessors (Convenience)
func Debug(ctx context.Context, msg string, args ...any) { slog.DebugContext(ctx, msg, args...) }
func Info(ctx context.Context, msg string, args ...any)  { slog.InfoContext(ctx, msg, args...) }
func Warn(ctx context.Context, msg string, args ...any)  { slog.WarnContext(ctx, msg, args...) }
func Error(ctx context.Context, msg string, args ...any) { slog.ErrorContext(ctx, msg, args...) }
