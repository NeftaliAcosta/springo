package event

import "github.com/NeftaliAcosta/springo/framework/config"

// EventProperties defines the event bus configuration in application.yaml
type EventProperties struct {
	Enabled     bool                  `yaml:"enabled"`
	DLQ         DLQProperties         `yaml:"dlq"`
	Concurrency ConcurrencyProperties `yaml:"concurrency"`
	Outbox      OutboxProperties      `yaml:"outbox"`
}

type OutboxProperties struct {
	Enabled      bool   `yaml:"enabled"`
	CleanUp      bool   `yaml:"cleanup"`
	PollInterval string `yaml:"poll-interval"`
}

type DLQProperties struct {
	Enabled        bool     `yaml:"enabled"`
	MaxRetries     int      `yaml:"max-retries"`
	RetryIntervals []string `yaml:"retry-intervals"` // e.g. ["10s", "1m", "5m"]
	Store          string   `yaml:"store"`           // e.g. "main-db"
}

type ConcurrencyProperties struct {
	Enabled         bool   `yaml:"enabled"`
	PoolSize        int    `yaml:"pool-size"`
	QueueCapacity   int    `yaml:"queue-capacity"`
	RejectionPolicy string `yaml:"rejection-policy"` // "block", "fallback", "discard"
}

func init() {
	// Register the event properties under spring.event
	props := &EventProperties{
		Enabled: true,
		DLQ: DLQProperties{
			Enabled:        true,
			MaxRetries:     3,
			RetryIntervals: []string{"30s", "1m", "5m"},
		},
		Concurrency: ConcurrencyProperties{
			Enabled:         true,
			PoolSize:        10,
			QueueCapacity:   100,
			RejectionPolicy: "block",
		},
	}
	config.RegisterProperties("spring.event", props)
}
