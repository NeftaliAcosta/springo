package response

import "github.com/NeftaliAcosta/springo/framework/web"

// HealthResponseDTO represents the health status of the application
type HealthResponseDTO struct {
	Status     web.HealthStatus               `json:"status"`
	Uptime     string                         `json:"uptime"`
	Components map[string]web.ComponentHealth `json:"components,omitempty"`
	System     *web.SystemMetrics             `json:"system,omitempty"`
}
