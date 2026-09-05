package logging

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSubsystemFilterHandler_FrameworkWarnAppInfo(t *testing.T) {
	var buf bytes.Buffer
	appLevel := &slog.LevelVar{}
	appLevel.Set(slog.LevelInfo)

	frameworkLevel := &slog.LevelVar{}
	frameworkLevel.Set(slog.LevelWarn)

	baseHandler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	filterHandler := NewSubsystemFilterHandler(baseHandler, appLevel, frameworkLevel)
	logger := slog.New(filterHandler)

	fwLogger := logger.With(slog.String(SubsystemKey, FrameworkSubsystem))

	// 1. Framework INFO log should be suppressed
	fwLogger.Info("framework info message")
	assert.Empty(t, buf.String())

	// 2. Framework WARN log should be emitted
	buf.Reset()
	fwLogger.Warn("framework warn message")
	assert.Contains(t, buf.String(), "framework warn message")

	// 3. Framework ERROR log should be emitted
	buf.Reset()
	fwLogger.Error("framework error message")
	assert.Contains(t, buf.String(), "framework error message")

	// 4. App INFO log should be emitted
	buf.Reset()
	logger.Info("application info message")
	assert.Contains(t, buf.String(), "application info message")
}

func TestSubsystemFilterHandler_FrameworkOff(t *testing.T) {
	var buf bytes.Buffer
	appLevel := &slog.LevelVar{}
	appLevel.Set(slog.LevelInfo)

	frameworkLevel := &slog.LevelVar{}
	frameworkLevel.Set(LevelOff)

	baseHandler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	filterHandler := NewSubsystemFilterHandler(baseHandler, appLevel, frameworkLevel)
	logger := slog.New(filterHandler)

	fwLogger := logger.With(slog.String(SubsystemKey, FrameworkSubsystem))

	// All framework logs should be suppressed under OFF
	fwLogger.Info("fw info")
	fwLogger.Warn("fw warn")
	fwLogger.Error("fw error")
	assert.Empty(t, buf.String())

	// App logs continue normally
	logger.Info("app info message")
	assert.Contains(t, buf.String(), "app info message")
}

func TestSubsystemFilterHandler_DynamicLevels(t *testing.T) {
	props := &LoggingProperties{
		Level:          "INFO",
		FrameworkLevel: "WARN",
		Format:         "text",
	}
	Initialize(props)

	assert.Equal(t, "INFO", GetLevel())
	assert.Equal(t, "WARN", GetFrameworkLevel())

	SetFrameworkLevel("DEBUG")
	assert.Equal(t, "DEBUG", GetFrameworkLevel())

	SetFrameworkLevel("OFF")
	assert.Equal(t, "OFF", GetFrameworkLevel())

	SetLevel("DEBUG")
	assert.Equal(t, "DEBUG", GetLevel())
}

func TestLoggingProperties_ShowBanner(t *testing.T) {
	p1 := &LoggingProperties{}
	assert.True(t, p1.IsBannerEnabled())

	bFalse := false
	p2 := &LoggingProperties{ShowBanner: &bFalse}
	assert.False(t, p2.IsBannerEnabled())

	bTrue := true
	p3 := &LoggingProperties{ShowBanner: &bTrue}
	assert.True(t, p3.IsBannerEnabled())
}

func TestContextHandler_Tracing(t *testing.T) {
	var buf bytes.Buffer
	baseHandler := slog.NewTextHandler(&buf, &slog.HandlerOptions{})
	ctxHandler := &ContextHandler{Handler: baseHandler}
	logger := slog.New(ctxHandler)

	ctx := context.WithValue(context.Background(), TraceIDKey, "trace-xyz-123")
	ctx = context.WithValue(ctx, SpanIDKey, "span-abc-456")

	logger.InfoContext(ctx, "traced operation")
	output := buf.String()

	assert.True(t, strings.Contains(output, "trace_id=trace-xyz-123"))
	assert.True(t, strings.Contains(output, "span_id=span-abc-456"))
}
