package logging

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// TraceIDKey must match the one defined in framework/web/tracing.go
var (
	TraceIDKey        = "springo_trace_id"
	SpanIDKey         = "springo_span_id"
	currentLevel      = &slog.LevelVar{}
	frameworkLevelVar = &slog.LevelVar{}
)

// ContextHandler is a custom slog handler that automatically adds trace_id and span_id from context
type ContextHandler struct {
	slog.Handler
}

// Handle appends trace_id and span_id from context if present.
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

// Initialize setups the global slog logger based on properties.
func Initialize(props *LoggingProperties) {
	if props == nil {
		props = &LoggingProperties{Level: "INFO", FrameworkLevel: "INFO", Format: "text"}
	}

	appLvl := props.GetSlogLevel()
	currentLevel.Set(appLvl)

	fwLvl := props.GetFrameworkSlogLevel()
	frameworkLevelVar.Set(fwLvl)

	opts := &slog.HandlerOptions{
		Level: slog.LevelDebug, // Base handler accepts all; SubsystemFilterHandler filters dynamically
	}

	var baseHandler slog.Handler
	if strings.ToLower(props.Format) == "json" {
		baseHandler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		baseHandler = slog.NewTextHandler(os.Stdout, opts)
	}

	// 1. Wrap with SubsystemFilterHandler to support granular app vs framework levels
	filterHandler := NewSubsystemFilterHandler(baseHandler, currentLevel, frameworkLevelVar)

	// 2. Wrap with ContextHandler to support tracing (trace_id, span_id)
	logger := slog.New(&ContextHandler{Handler: filterHandler})
	slog.SetDefault(logger)
}

// SetLevel changes the active application logging level dynamically.
func SetLevel(levelStr string) {
	lvl := ParseLevel(levelStr, slog.LevelInfo)
	currentLevel.Set(lvl)
}

// SetFrameworkLevel changes the active framework logging level dynamically.
func SetFrameworkLevel(levelStr string) {
	if strings.ToUpper(strings.TrimSpace(levelStr)) == "OFF" {
		frameworkLevelVar.Set(LevelOff)
		return
	}
	lvl := ParseLevel(levelStr, slog.LevelInfo)
	frameworkLevelVar.Set(lvl)
}

// GetLevel returns the string representation of the active logging level.
func GetLevel() string {
	return currentLevel.Level().String()
}

// GetFrameworkLevel returns the string representation of the active framework logging level.
func GetFrameworkLevel() string {
	if frameworkLevelVar.Level() == LevelOff {
		return "OFF"
	}
	return frameworkLevelVar.Level().String()
}

// Global Logger Accessors (Convenience)
func Debug(ctx context.Context, msg string, args ...any) { slog.DebugContext(ctx, msg, args...) }
func Info(ctx context.Context, msg string, args ...any)  { slog.InfoContext(ctx, msg, args...) }
func Warn(ctx context.Context, msg string, args ...any)  { slog.WarnContext(ctx, msg, args...) }
func Error(ctx context.Context, msg string, args ...any) { slog.ErrorContext(ctx, msg, args...) }
