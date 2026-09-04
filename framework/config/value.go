package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	activeLoaderMu sync.RWMutex
	activeLoader   *ConfigLoader
)

// SetActiveLoader sets the global active config loader instance for direct property lookups.
func SetActiveLoader(loader *ConfigLoader) {
	activeLoaderMu.Lock()
	defer activeLoaderMu.Unlock()
	activeLoader = loader
}

// GetActiveLoader returns the current active config loader.
func GetActiveLoader() *ConfigLoader {
	activeLoaderMu.RLock()
	defer activeLoaderMu.RUnlock()
	return activeLoader
}

// ResetActiveLoader clears the current active config loader.
func ResetActiveLoader() {
	activeLoaderMu.Lock()
	defer activeLoaderMu.Unlock()
	activeLoader = nil
}

// GetString retrieves a string property by dot-path or returns defaultVal if missing or empty.
func GetString(path, defaultVal string) string {
	raw, found := resolvePropertyPath(path)
	if !found || raw == nil {
		return defaultVal
	}

	strVal := coerceToString(raw)
	if strVal == "" {
		return defaultVal
	}
	return strVal
}

// GetInt retrieves an integer property by dot-path or returns defaultVal if missing or invalid.
func GetInt(path string, defaultVal int) int {
	raw, found := resolvePropertyPath(path)
	if !found || raw == nil {
		return defaultVal
	}

	intVal, ok := coerceToInt(raw)
	if !ok {
		return defaultVal
	}
	return intVal
}

// GetBool retrieves a boolean property by dot-path or returns defaultVal if missing or invalid.
func GetBool(path string, defaultVal bool) bool {
	raw, found := resolvePropertyPath(path)
	if !found || raw == nil {
		return defaultVal
	}

	boolVal, ok := coerceToBool(raw)
	if !ok {
		return defaultVal
	}
	return boolVal
}

// GetDuration retrieves a time.Duration property by dot-path or returns defaultVal if missing.
func GetDuration(path string, defaultVal time.Duration) time.Duration {
	raw, found := resolvePropertyPath(path)
	if !found || raw == nil {
		return defaultVal
	}

	durVal, ok := coerceToDuration(raw)
	if !ok {
		return defaultVal
	}
	return durVal
}

// GetValue retrieves a typed property by dot-path with generic fallback.
func GetValue[T any](path string, defaultVal T) T {
	raw, found := resolvePropertyPath(path)
	if !found || raw == nil {
		return defaultVal
	}

	if typed, ok := raw.(T); ok {
		return typed
	}

	marshaled, err := yaml.Marshal(raw)
	if err != nil {
		return defaultVal
	}

	var target T
	if err := yaml.Unmarshal(marshaled, &target); err != nil {
		return defaultVal
	}
	return target
}

// ResolvePropertyPath navigates the loaded YAML data tree and environment variables.
func resolvePropertyPath(path string) (any, bool) {
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" {
		return nil, false
	}

	loader := GetActiveLoader()
	if loader != nil && loader.Data != nil {
		if val, found := lookupNestedKey(loader.Data, cleanPath); found {
			return val, true
		}
	}

	return lookupEnvironmentFallback(cleanPath)
}

func lookupNestedKey(data map[string]interface{}, path string) (any, bool) {
	if directVal, exists := data[path]; exists {
		return directVal, true
	}

	parts := strings.Split(path, ".")
	var current interface{} = data

	for _, part := range parts {
		currentMap, ok := current.(map[string]interface{})
		if !ok {
			return nil, false
		}
		nextVal, found := currentMap[part]
		if !found {
			return nil, false
		}
		current = nextVal
	}

	return current, true
}

func lookupEnvironmentFallback(path string) (any, bool) {
	envKey := strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(path, ".", "_"), "-", "_"))
	if envVal, found := os.LookupEnv(envKey); found {
		return envVal, true
	}

	if envVal, found := os.LookupEnv(path); found {
		return envVal, true
	}

	return nil, false
}

func coerceToString(raw any) string {
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

func coerceToInt(raw any) (int, bool) {
	switch v := raw.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(v))
		return parsed, err == nil
	default:
		return 0, false
	}
}

func coerceToBool(raw any) (bool, bool) {
	switch v := raw.(type) {
	case bool:
		return v, true
	case int:
		return v != 0, true
	case string:
		return parseBoolString(v)
	default:
		return false, false
	}
}

func parseBoolString(s string) (bool, bool) {
	normalized := strings.ToLower(strings.TrimSpace(s))
	switch normalized {
	case "true", "1", "yes", "on", "enabled":
		return true, true
	case "false", "0", "no", "off", "disabled":
		return false, true
	default:
		return false, false
	}
}

func coerceToDuration(raw any) (time.Duration, bool) {
	switch v := raw.(type) {
	case time.Duration:
		return v, true
	case int:
		return time.Duration(v) * time.Millisecond, true
	case int64:
		return time.Duration(v) * time.Millisecond, true
	case string:
		dur, err := time.ParseDuration(strings.TrimSpace(v))
		return dur, err == nil
	default:
		return 0, false
	}
}
