package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var envVarRegex = regexp.MustCompile(`\$\{([A-Za-z0-9_]+)(?::([^}]+))?\}`)

func expandEnv(content []byte) []byte {
	return envVarRegex.ReplaceAllFunc(content, func(match []byte) []byte {
		submatches := envVarRegex.FindSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		varName := string(submatches[1])
		val, found := os.LookupEnv(varName)
		if found {
			return []byte(val)
		}
		if len(submatches) > 2 && len(submatches[2]) > 0 {
			return submatches[2]
		}
		return []byte("")
	})
}

// ConfigLoader handles loading and merging YAML configuration files
type ConfigLoader struct {
	ActiveProfile string
	Data          map[string]interface{}
	BaseDir       string
}

// NewConfigLoader initializes a new loader and detects the active profile
func NewConfigLoader() *ConfigLoader {
	profile := os.Getenv("SPRINGO_PROFILES_ACTIVE")
	if profile == "" {
		profile = "default"
	}
	return &ConfigLoader{
		ActiveProfile: profile,
		Data:          make(map[string]interface{}),
		BaseDir:       "resources",
	}
}

// WithBaseDir configures the directory base path where config files reside
func (l *ConfigLoader) WithBaseDir(dir string) *ConfigLoader {
	l.BaseDir = dir
	return l
}

// Load reads application.yaml (base) and then merges the profile-specific yaml file if active
func (l *ConfigLoader) Load() error {
	// 1. Always load the base configuration (optional)
	baseFile := "application.yaml"
	if err := l.loadYamlFile(baseFile); err != nil {
		if os.IsNotExist(err) {
			log.Printf("[SprinGo] Base configuration %s not found, proceeding with internal defaults", baseFile)
		} else {
			return fmt.Errorf("failed to load base configuration file %s: %v", baseFile, err)
		}
	}

	// 2. Load and merge profile-specific configuration if active
	if l.ActiveProfile != "default" {
		profileFile := fmt.Sprintf("application-%s.yaml", l.ActiveProfile)
		if err := l.loadYamlFile(profileFile); err != nil {
			log.Printf("[SprinGo] Warning: Profile-specific file %s not found or unreadable: %v", profileFile, err)
		} else {
			log.Printf("[SprinGo] Active profile configuration merged: %s", profileFile)
		}
	}

	log.Printf("[SprinGo] Configuration initialized (Profile: %s)", l.ActiveProfile)
	return nil
}

func (l *ConfigLoader) loadYamlFile(filename string) error {
	path := filepath.Join(l.BaseDir, filename)
	content, err := os.ReadFile(path)
	if err != nil {
		return err // File must exist
	}

	tempData := make(map[string]interface{})
	// Expand environment variables
	expandedContent := expandEnv(content)
	if err := yaml.Unmarshal(expandedContent, &tempData); err != nil {
		return fmt.Errorf("error parsing %s: %v", filename, err)
	}

	// Deep merge into main Data map
	l.deepMerge(l.Data, tempData)

	return nil
}

func (l *ConfigLoader) deepMerge(dest, src map[string]interface{}) {
	for k, v := range src {
		if srcMap, ok := v.(map[string]interface{}); ok {
			if destMap, ok := dest[k].(map[string]interface{}); ok {
				l.deepMerge(destMap, srcMap)
				continue
			}
		}
		dest[k] = v
	}
}

// Bind maps the loaded configuration to a provided struct
func (l *ConfigLoader) Bind(dest interface{}) error {
	marshaled, err := yaml.Marshal(l.Data)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(marshaled, dest)
}

// BindPrefix maps a specific sub-block of the configuration to a struct based on a prefix
func (l *ConfigLoader) BindPrefix(prefix string, dest interface{}) error {
	keys := strings.Split(prefix, ".")
	var current interface{} = l.Data

	for _, key := range keys {
		if m, ok := current.(map[string]interface{}); ok {
			if next, found := m[key]; found {
				current = next
			} else {
				return nil // Prefix not found
			}
		} else {
			return nil // Not a map
		}
	}

	marshaled, err := yaml.Marshal(current)
	if err != nil {
		return err
	}

	return yaml.Unmarshal(marshaled, dest)
}
