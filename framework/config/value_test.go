package config_test

import (
	"os"
	"testing"
	"time"

	"github.com/NeftaliAcosta/springo/framework/config"
)

func setupTestLoader(data map[string]interface{}) *config.ConfigLoader {
	loader := config.NewConfigLoader()
	loader.Data = data
	config.SetActiveLoader(loader)
	return loader
}

func TestGetString(t *testing.T) {
	setupTestLoader(map[string]interface{}{
		"app": map[string]interface{}{
			"name": "SprinGoApp",
			"code": 12345,
		},
	})
	defer config.ResetActiveLoader()

	if got := config.GetString("app.name", "default"); got != "SprinGoApp" {
		t.Fatalf("expected 'SprinGoApp', got %q", got)
	}

	if got := config.GetString("app.code", "default"); got != "12345" {
		t.Fatalf("expected '12345', got %q", got)
	}

	if got := config.GetString("app.missing", "fallback"); got != "fallback" {
		t.Fatalf("expected 'fallback', got %q", got)
	}
}

func TestGetInt(t *testing.T) {
	setupTestLoader(map[string]interface{}{
		"server": map[string]interface{}{
			"port":    8080,
			"timeout": "60",
		},
	})
	defer config.ResetActiveLoader()

	if got := config.GetInt("server.port", 3000); got != 8080 {
		t.Fatalf("expected 8080, got %d", got)
	}

	if got := config.GetInt("server.timeout", 10); got != 60 {
		t.Fatalf("expected 60, got %d", got)
	}

	if got := config.GetInt("server.missing", 9000); got != 9000 {
		t.Fatalf("expected 9000, got %d", got)
	}
}

func TestGetBoolDirect(t *testing.T) {
	setupTestLoader(map[string]interface{}{
		"features": map[string]interface{}{
			"auth":    true,
			"tracing": 1,
		},
	})
	defer config.ResetActiveLoader()

	if got := config.GetBool("features.auth", false); !got {
		t.Fatalf("expected true for features.auth")
	}

	if got := config.GetBool("features.tracing", false); !got {
		t.Fatalf("expected true for 1")
	}
}

func TestGetBoolMetrics(t *testing.T) {
	setupTestLoader(map[string]interface{}{
		"features": map[string]interface{}{
			"metrics": "enabled",
		},
	})
	defer config.ResetActiveLoader()

	if got := config.GetBool("features.metrics", false); !got {
		t.Fatalf("expected true for 'enabled'")
	}
}

func TestGetBoolLegacy(t *testing.T) {
	setupTestLoader(map[string]interface{}{
		"features": map[string]interface{}{
			"legacy": "false",
		},
	})
	defer config.ResetActiveLoader()

	if got := config.GetBool("features.legacy", true); got {
		t.Fatalf("expected false for 'false'")
	}
}

func TestGetBoolProfiling(t *testing.T) {
	setupTestLoader(map[string]interface{}{
		"features": map[string]interface{}{
			"profiling": "off",
		},
	})
	defer config.ResetActiveLoader()

	if got := config.GetBool("features.profiling", true); got {
		t.Fatalf("expected false for 'off'")
	}
}

func TestGetBoolDefault(t *testing.T) {
	setupTestLoader(map[string]interface{}{})
	defer config.ResetActiveLoader()

	if got := config.GetBool("features.missing", true); !got {
		t.Fatalf("expected default true for missing key")
	}
}

func TestGetDuration(t *testing.T) {
	setupTestLoader(map[string]interface{}{
		"timeouts": map[string]interface{}{
			"read":  "5s",
			"write": 1000,
		},
	})
	defer config.ResetActiveLoader()

	if got := config.GetDuration("timeouts.read", time.Second); got != 5*time.Second {
		t.Fatalf("expected 5s, got %v", got)
	}

	if got := config.GetDuration("timeouts.write", time.Second); got != 1000*time.Millisecond {
		t.Fatalf("expected 1000ms, got %v", got)
	}

	if got := config.GetDuration("timeouts.missing", 2*time.Minute); got != 2*time.Minute {
		t.Fatalf("expected 2m, got %v", got)
	}
}

func TestGetValue(t *testing.T) {
	setupTestLoader(map[string]interface{}{
		"database": map[string]interface{}{
			"hosts": []interface{}{"db1.local", "db2.local"},
			"pool": map[string]interface{}{
				"max_open": 50,
			},
		},
	})
	defer config.ResetActiveLoader()

	type poolConfig struct {
		MaxOpen int `yaml:"max_open"`
	}

	pool := config.GetValue[poolConfig]("database.pool", poolConfig{MaxOpen: 10})
	if pool.MaxOpen != 50 {
		t.Fatalf("expected max_open 50, got %d", pool.MaxOpen)
	}

	hosts := config.GetValue[[]string]("database.hosts", nil)
	if len(hosts) != 2 || hosts[0] != "db1.local" {
		t.Fatalf("unexpected hosts: %v", hosts)
	}
}

func TestEnvironmentFallback(t *testing.T) {
	_ = os.Setenv("APP_OBSERVABILITY_KEY", "env-secret-value-123")
	defer func() { _ = os.Unsetenv("APP_OBSERVABILITY_KEY") }()

	setupTestLoader(make(map[string]interface{}))
	defer config.ResetActiveLoader()

	got := config.GetString("app.observability.key", "default")
	if got != "env-secret-value-123" {
		t.Fatalf("expected 'env-secret-value-123', got %q", got)
	}
}
