package web

import (
	"fmt"
	"github.com/NeftaliAcosta/springo/framework/config"
	"github.com/NeftaliAcosta/springo/framework/database"
	"github.com/NeftaliAcosta/springo/framework/ioc"
	"runtime"
	"sync"
	"time"

	"gorm.io/gorm"
)

// HealthStatus represents the state of a component or the application
type HealthStatus string

const (
	StatusUp       HealthStatus = "UP"
	StatusDown     HealthStatus = "DOWN"
	StatusDegraded HealthStatus = "DEGRADED"
	StatusUnknown  HealthStatus = "UNKNOWN"
)

// ComponentHealth holds the health information for a single indicator
type ComponentHealth struct {
	Status  HealthStatus           `json:"status"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// HealthInfo represents the complete health state of the application
type HealthInfo struct {
	Status     HealthStatus               `json:"status"`
	Uptime     string                     `json:"uptime"`
	Components map[string]ComponentHealth `json:"components,omitempty"`
	System     *SystemMetrics             `json:"system,omitempty"`
}

// SystemMetrics holds low-level resource information
type SystemMetrics struct {
	Goroutines int    `json:"goroutines"`
	Memory     string `json:"memory_usage"`
	GoVersion  string `json:"go_version"`
	Platform   string `json:"platform"`
}

// HealthIndicator is the interface that all health contributors must implement
type HealthIndicator interface {
	Name() string
	Health() ComponentHealth
}

var (
	appStartTime time.Time
	registry     []HealthIndicator
	registryMu   sync.RWMutex
)

// SetStartTime records when the framework finished bootstrapping
func SetStartTime(t time.Time) {
	appStartTime = t
}

// RegisterHealthIndicator adds a new indicator to the registry
func RegisterHealthIndicator(indicator HealthIndicator) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry = append(registry, indicator)
}

// InitializeHealthIndicators setups standard indicators based on config
func InitializeHealthIndicators() {
	props := config.Get[HealthProperties]()
	if props == nil {
		return
	}

	if props.DiskSpace.Enabled {
		RegisterHealthIndicator(&DiskSpaceHealthIndicator{})
	}
}

// GetHealthInfo collects data from all registered and configured indicators
func GetHealthInfo() HealthInfo {
	props := config.Get[HealthProperties]()
	if props == nil {
		props = &HealthProperties{} // Default empty
	}

	finalStatus := StatusUp
	components := make(map[string]ComponentHealth)

	// 1. Process Registered Indicators (Opt-in via registry)
	registryMu.RLock()
	for _, indicator := range registry {
		health := indicator.Health()
		components[indicator.Name()] = health
		finalStatus = aggregateStatus(finalStatus, health.Status)
	}
	registryMu.RUnlock()

	// 2. Process DataSources (Opt-in via YAML health-check: true)
	if props.ShowComponents {
		dbComponents, dbStatus := collectDatabaseHealth()
		for k, v := range dbComponents {
			components[k] = v
		}
		finalStatus = aggregateStatus(finalStatus, dbStatus)
	}

	// 3. System Metrics (Opt-in via YAML show-system: true)
	var systemMetrics *SystemMetrics
	if props.ShowSystem {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		systemMetrics = &SystemMetrics{
			Goroutines: runtime.NumGoroutine(),
			Memory:     fmt.Sprintf("%.2f MB", float64(m.Alloc)/1024/1024),
			GoVersion:  runtime.Version(),
			Platform:   fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		}
	}

	uptime := "0s"
	if !appStartTime.IsZero() {
		uptime = time.Since(appStartTime).Round(time.Second).String()
	}

	// Clean up components if show-components is false and no custom indicators registered
	var finalComponents map[string]ComponentHealth
	if props.ShowComponents && len(components) > 0 {
		finalComponents = components
	}

	return HealthInfo{
		Status:     finalStatus,
		Uptime:     uptime,
		Components: finalComponents,
		System:     systemMetrics,
	}
}

func aggregateStatus(current, new HealthStatus) HealthStatus {
	// Priority: DOWN > DEGRADED > UP > UNKNOWN
	if current == StatusDown || new == StatusDown {
		return StatusDown
	}
	if current == StatusDegraded || new == StatusDegraded {
		return StatusDegraded
	}
	if new == StatusUp {
		return StatusUp
	}
	return current
}

func collectDatabaseHealth() (map[string]ComponentHealth, HealthStatus) {
	components := make(map[string]ComponentHealth)
	finalStatus := StatusUp

	// Primary DataSource
	primaryProps := config.Get[database.DataSourceProperties]()
	if primaryProps != nil && primaryProps.HealthCheck {
		db := ioc.GetContainer().GetDB()
		health := checkDB("primary", db)
		components["db.primary"] = health
		finalStatus = aggregateStatus(finalStatus, health.Status)
	}

	// Additional DataSources
	additionalProps := config.Get[database.AdditionalDataSources]()
	if additionalProps != nil {
		for name, props := range *additionalProps {
			if props.HealthCheck {
				bean := ioc.GetContainer().GetBean(name)
				if db, ok := bean.(*gorm.DB); ok {
					health := checkDB(name, db)
					components["db."+name] = health
					finalStatus = aggregateStatus(finalStatus, health.Status)
				}
			}
		}
	}

	return components, finalStatus
}

func checkDB(name string, db *gorm.DB) ComponentHealth {
	if db == nil {
		return ComponentHealth{Status: StatusDown, Details: map[string]interface{}{"error": "database connection is nil"}}
	}

	sqlDB, err := db.DB()
	if err != nil {
		return ComponentHealth{Status: StatusDown, Details: map[string]interface{}{"error": err.Error()}}
	}

	start := time.Now()
	err = sqlDB.Ping()
	duration := time.Since(start).String()

	if err != nil {
		return ComponentHealth{Status: StatusDown, Details: map[string]interface{}{"error": err.Error()}}
	}

	// Enterprise addition: Pool stats
	stats := sqlDB.Stats()

	return ComponentHealth{
		Status: StatusUp,
		Details: map[string]interface{}{
			"database":      name,
			"ping_time":     duration,
			"max_open":      stats.MaxOpenConnections,
			"open":          stats.OpenConnections,
			"in_use":        stats.InUse,
			"idle":          stats.Idle,
			"wait_count":    stats.WaitCount,
			"wait_duration": stats.WaitDuration.String(),
		},
	}
}

func pingDB(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
}
