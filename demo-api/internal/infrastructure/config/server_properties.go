package config

import (
	"github.com/NeftaliAcosta/springo/framework/config"
)

// ServerProperties holds the web server configuration
type ServerProperties struct {
	Port int    `yaml:"port"`
	Name string `yaml:"name"`
}

func init() {
	// Register the struct to be filled from the "server" YAML block
	config.RegisterProperties("server", &ServerProperties{})
}
