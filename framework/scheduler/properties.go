package scheduler

import "github.com/NeftaliAcosta/springo/framework/config"

// SchedulerProperties maps the spring.scheduler block in application.yaml
type SchedulerProperties struct {
	Enabled bool               `yaml:"enabled"`
	Jobs    map[string]JobConf `yaml:"jobs"`
}

// JobConf defines the configuration for a single scheduled task
type JobConf struct {
	Cron         string         `yaml:"cron"`
	FixedRate    string         `yaml:"fixed-rate"`
	FixedDelay   string         `yaml:"fixed-delay"`
	RunOnStartup bool           `yaml:"run-on-startup"`
	Priority     int            `yaml:"priority"`
	Critical     bool           `yaml:"critical"`
	Enabled      bool           `yaml:"enabled"`
	Lock         LockProperties `yaml:"lock"`
}

type LockProperties struct {
	Enabled        bool   `yaml:"enabled"`
	LockAtMostFor  string `yaml:"lock-at-most-for"`  // e.g. "10m"
	LockAtLeastFor string `yaml:"lock-at-least-for"` // e.g. "30s"
}

func init() {
	// Register properties to be automatically filled from YAML
	config.RegisterProperties("spring.scheduler", &SchedulerProperties{
		Enabled: false, // Disabled by default
	})
}
