package logging

import (
	"log/slog"
	"strings"

	"github.com/NeftaliAcosta/springo/framework/config"
)

// LevelOff is a custom sentinel slog.Level representing fully disabled logging.
const LevelOff slog.Level = slog.Level(1000)

// LoggingProperties defines the logging configuration in application.yaml.
type LoggingProperties struct {
	Level          string            `yaml:"level"`           // Global level: DEBUG, INFO, WARN, ERROR
	FrameworkLevel string            `yaml:"framework-level"` // Framework level: OFF, ERROR, WARN, INFO, DEBUG
	Format         string            `yaml:"format"`          // text, json
	Levels         map[string]string `yaml:"levels"`          // Per-package levels
	Enabled        bool              `yaml:"enabled"`         // Master switch
	ShowBanner     *bool             `yaml:"show-banner"`     // Independent control for ASCII banner
}

func init() {
	// Register logging properties under "spring.logging"
	config.RegisterProperties("spring.logging", &LoggingProperties{})
}

// GetSlogLevel returns the application-level slog.Level.
func (p *LoggingProperties) GetSlogLevel() slog.Level {
	return ParseLevel(p.Level, slog.LevelInfo)
}

// GetFrameworkSlogLevel returns the framework-specific slog.Level or LevelOff.
func (p *LoggingProperties) GetFrameworkSlogLevel() slog.Level {
	if p.FrameworkLevel == "" {
		return slog.LevelInfo
	}
	if strings.ToUpper(strings.TrimSpace(p.FrameworkLevel)) == "OFF" {
		return LevelOff
	}
	return ParseLevel(p.FrameworkLevel, slog.LevelInfo)
}

// IsBannerEnabled returns true if the startup ASCII banner should be displayed.
func (p *LoggingProperties) IsBannerEnabled() bool {
	if p.ShowBanner == nil {
		return true
	}
	return *p.ShowBanner
}

// ParseLevel converts a string level name to slog.Level with fallback.
func ParseLevel(levelStr string, fallback slog.Level) slog.Level {
	switch strings.ToUpper(strings.TrimSpace(levelStr)) {
	case "DEBUG":
		return slog.LevelDebug
	case "INFO":
		return slog.LevelInfo
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return fallback
	}
}
