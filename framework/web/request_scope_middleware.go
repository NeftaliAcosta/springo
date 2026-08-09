package web

import (
	"context"
	"github.com/NeftaliAcosta/springo/framework/ioc"
	"net/http"
)

// RequestScopeMiddleware establishes the RequestRegistry for the lifecycle of an HTTP request.
func RequestScopeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		container := ioc.GetContainer()
		registry := container.CreateRequestRegistry()
		defer container.DestroyRequestRegistry(registry)

		// Set default HTTP objects in request scope so they are available as beans if needed
		registry.Set("requestContext", r.Context())
		registry.Set("httpRequest", r)

		// Put the registry in the context
		ctx := context.WithValue(r.Context(), ioc.RegistryKey(), registry)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
