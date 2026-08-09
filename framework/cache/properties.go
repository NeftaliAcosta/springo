package cache

import "github.com/NeftaliAcosta/springo/framework/config"

// CacheProperties defines the cache configuration in application.yaml
type CacheProperties struct {
	Type    string                  `yaml:"type"`    // memory (default), redis
	Enabled bool                    `yaml:"enabled"` // globally enable/disable cache
	Configs map[string]RegionConfig `yaml:"configs"` // per-region settings
}

type RegionConfig struct {
	Type string `yaml:"type"` // overrides global type for this region
	TTL  string `yaml:"ttl"`  // e.g. "10m", "1h"
}

func init() {
	// Register the cache properties under spring.cache
	config.RegisterProperties("spring.cache", &CacheProperties{})
}
