package logging

import (
	"context"
	"log/slog"
)

const (
	// SubsystemKey is the slog attribute key used to identify subsystems.
	SubsystemKey = "subsystem"

	// FrameworkSubsystem is the attribute value identifying SprinGo internal logs.
	FrameworkSubsystem = "springo"
)

// SubsystemFilterHandler routes and filters logs based on whether they originate from the framework or the application.
type SubsystemFilterHandler struct {
	next           slog.Handler
	appLevel       *slog.LevelVar
	frameworkLevel *slog.LevelVar
	isFramework    bool
}

// NewSubsystemFilterHandler creates a new SubsystemFilterHandler wrapping the provided handler.
func NewSubsystemFilterHandler(
	next slog.Handler,
	appLevel *slog.LevelVar,
	frameworkLevel *slog.LevelVar,
) *SubsystemFilterHandler {
	return &SubsystemFilterHandler{
		next:           next,
		appLevel:       appLevel,
		frameworkLevel: frameworkLevel,
		isFramework:    false,
	}
}

// Enabled performs a fast check to see if logging is permitted at the specified level.
func (h *SubsystemFilterHandler) Enabled(ctx context.Context, level slog.Level) bool {
	// If this handler instance is pre-tagged with framework subsystem, evaluate against frameworkLevel
	if h.isFramework {
		return level >= h.frameworkLevel.Level()
	}

	// For general loggers, permit if either app or framework level threshold is met
	return level >= h.appLevel.Level() || level >= h.frameworkLevel.Level()
}

// Handle inspects the record to determine origin and enforces the appropriate log level.
func (h *SubsystemFilterHandler) Handle(ctx context.Context, r slog.Record) error {
	isFrameworkRecord := h.isFramework

	if !isFrameworkRecord {
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == SubsystemKey && a.Value.String() == FrameworkSubsystem {
				isFrameworkRecord = true
				return false
			}
			return true
		})
	}

	if isFrameworkRecord {
		if r.Level < h.frameworkLevel.Level() {
			return nil
		}
	} else {
		if r.Level < h.appLevel.Level() {
			return nil
		}
	}

	return h.next.Handle(ctx, r)
}

// WithAttrs returns a new SubsystemFilterHandler with additional attributes.
func (h *SubsystemFilterHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	isFramework := h.isFramework
	for _, a := range attrs {
		if a.Key == SubsystemKey && a.Value.String() == FrameworkSubsystem {
			isFramework = true
			break
		}
	}

	return &SubsystemFilterHandler{
		next:           h.next.WithAttrs(attrs),
		appLevel:       h.appLevel,
		frameworkLevel: h.frameworkLevel,
		isFramework:    isFramework,
	}
}

// WithGroup returns a new SubsystemFilterHandler with the group applied to the underlying handler.
func (h *SubsystemFilterHandler) WithGroup(name string) slog.Handler {
	return &SubsystemFilterHandler{
		next:           h.next.WithGroup(name),
		appLevel:       h.appLevel,
		frameworkLevel: h.frameworkLevel,
		isFramework:    h.isFramework,
	}
}

// FrameworkLogger returns an *slog.Logger pre-configured with the "subsystem": "springo" attribute.
func FrameworkLogger() *slog.Logger {
	return slog.Default().With(slog.String(SubsystemKey, FrameworkSubsystem))
}
