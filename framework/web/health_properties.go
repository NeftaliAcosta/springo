package web

import "github.com/NeftaliAcosta/springo/framework/config"

// HealthProperties defines the management configuration in application.yaml
type HealthProperties struct {
	ShowSystem     bool                `yaml:"show-system"`
	ShowComponents bool                `yaml:"show-components"`
	DiskSpace      DiskSpaceProperties `yaml:"diskspace"`
}

type DiskSpaceProperties struct {
	Enabled   bool   `yaml:"enabled"`
	Path      string `yaml:"path"`
	Threshold string `yaml:"threshold"` // e.g. "100MB"
}

func init() {
	// Register the management properties under management.health
	config.RegisterProperties("management.health", &HealthProperties{})
}
