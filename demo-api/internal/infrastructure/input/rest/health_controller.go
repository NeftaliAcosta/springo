package rest

import (
	"context"
	"github.com/NeftaliAcosta/springo/demo-api/internal/domain/port/in"
	"github.com/NeftaliAcosta/springo/framework/web"

	"github.com/go-chi/chi/v5"
)

// HealthController handles system monitoring
type HealthController struct {
	healthUseCase in.HealthUseCase
}

// init registers the controller automatically
func init() {
	web.Register(func(r chi.Router) {
		service := web.GetService[in.HealthUseCase]("HealthService")
		NewHealthController(r, service)
	})
}

// NewHealthController initializes health routes
func NewHealthController(r chi.Router, s in.HealthUseCase) {
	c := &HealthController{healthUseCase: s}
	r.Route("/actuator", func(r chi.Router) {
		r.Get("/health", web.Dispatch(c.checkHealth))
		r.Get("/info", web.Dispatch(c.getInfo))
	})
}

// @Summary System Health Check
// @Tags system
// @Router /actuator/health [get]
func (c *HealthController) checkHealth(ctx context.Context, _ interface{}) (any, error) {
	return c.healthUseCase.CheckStatus(ctx), nil
}

// @Summary System Information
// @Tags system
// @Router /actuator/info [get]
func (c *HealthController) getInfo(ctx context.Context, _ interface{}) (any, error) {
	return c.healthUseCase.GetInfo(ctx), nil
}
