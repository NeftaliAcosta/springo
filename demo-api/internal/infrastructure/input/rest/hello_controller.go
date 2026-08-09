package rest

import (
	"context"
	"github.com/NeftaliAcosta/springo/framework/web"

	"github.com/go-chi/chi/v5"
)

// HelloController for system health checks
type HelloController struct{}

// init registers the controller automatically
func init() {
	web.Register(func(r chi.Router) {
		NewHelloController(r)
	})
}

// NewHelloController registers system-level endpoints
func NewHelloController(r chi.Router) {
	c := &HelloController{}
	r.Get("/hello", web.Dispatch(c.hello))
}

// @Summary System Welcome
// @Router /hello [get]
func (c *HelloController) hello(ctx context.Context, _ interface{}) (any, error) {
	return map[string]string{
		"message": "Welcome to SprinGo - Hexagonal Architecture is active!",
		"status":  "success",
	}, nil
}
