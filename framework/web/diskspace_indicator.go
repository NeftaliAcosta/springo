package web

import (
	"fmt"
	"github.com/NeftaliAcosta/springo/framework/config"
	"runtime"
	"syscall"
)

// DiskSpaceHealthIndicator monitors the available storage
type DiskSpaceHealthIndicator struct{}

func (d *DiskSpaceHealthIndicator) Name() string {
	return "diskSpace"
}

func (d *DiskSpaceHealthIndicator) Health() ComponentHealth {
	props := config.Get[HealthProperties]()
	if props == nil || !props.DiskSpace.Enabled {
		return ComponentHealth{Status: StatusUnknown}
	}

	path := props.DiskSpace.Path
	if path == "" {
		path = "." // Default current dir
	}

	free, total, err := getDiskUsage(path)
	if err != nil {
		return ComponentHealth{
			Status:  StatusDown,
			Details: map[string]interface{}{"error": err.Error()},
		}
	}

	// Simple threshold check (hardcoded for now, could use threshold prop)
	status := StatusUp
	if free < (10 * 1024 * 1024) { // Less than 10MB
		status = StatusDown
	}

	return ComponentHealth{
		Status: status,
		Details: map[string]interface{}{
			"total": fmt.Sprintf("%.2f GB", float64(total)/(1024*1024*1024)),
			"free":  fmt.Sprintf("%.2f GB", float64(free)/(1024*1024*1024)),
			"path":  path,
		},
	}
}

func getDiskUsage(path string) (free uint64, total uint64, err error) {
	if runtime.GOOS == "windows" {
		return 0, 0, fmt.Errorf("disk health check not supported on windows yet")
	}

	var stat syscall.Statfs_t
	err = syscall.Statfs(path, &stat)
	if err != nil {
		return 0, 0, err
	}

	// Available blocks * size per block
	free = stat.Bavail * uint64(stat.Bsize)
	total = stat.Blocks * uint64(stat.Bsize)

	return free, total, nil
}

func init() {
	// We don't register it here yet to avoid forcing it if disabled.
	// But in a real enterprise app, we could check config during bootstrap and register.
}

// Ensure it implements interface
var _ HealthIndicator = (*DiskSpaceHealthIndicator)(nil)
