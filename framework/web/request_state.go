package web

import (
	"context"
	"net/http"
	"sync"

	"github.com/NeftaliAcosta/springo/framework/config"
	"github.com/NeftaliAcosta/springo/framework/security"
)

// RequestState aliases the canonical security request state for web users.
type RequestState = security.RequestState

var optimizationOnce sync.Once
var optimizationEnabled bool

func requestOptimizationEnabled() bool {
	optimizationOnce.Do(func() {
		props := config.Get[WebServerProperties]()
		optimizationEnabled = props != nil && props.RequestOptimization
	})
	return optimizationEnabled
}

// RequestStateFromContext returns optimized request state when enabled.
func RequestStateFromContext(ctx context.Context) (*RequestState, bool) {
	return security.RequestStateFromContext(ctx)
}

// RequestOptimizationMiddleware enables pooled request state before custom middleware.
func RequestOptimizationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !requestOptimizationEnabled() {
			next.ServeHTTP(writer, request)
			return
		}
		state := security.AcquireRequestState()
		ctx := security.WithRequestState(request.Context(), state)
		next.ServeHTTP(writer, request.WithContext(ctx))
		security.ReleaseRequestState(state)
	})
}
