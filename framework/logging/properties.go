package logging

import (
	"github.com/NeftaliAcosta/springo/framework/config"
	"log/slog"
)

// LoggingProperties defines the logging configuration in application.yaml
type LoggingProperties struct {
	Level   string            `yaml:"level"`   // Global level: DEBUG, INFO, WARN, ERROR
	Format  string            `yaml:"format"`  // text, json
	Levels  map[string]string `yaml:"levels"`  // Per-package levels
	Enabled bool              `yaml:"enabled"` // Master switch
}

func init() {
	// Register logging properties under "spring.logging"
	config.RegisterProperties("spring.logging", &LoggingProperties{})
}

func (p *LoggingProperties) GetSlogLevel() slog.Level {
	switch p.Level {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
