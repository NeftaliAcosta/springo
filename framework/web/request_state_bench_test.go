package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/NeftaliAcosta/springo/framework/security"
)

func BenchmarkLegacyRequestContext(b *testing.B) {
	handler := http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		_ = request.Context().Value(security.UserContextKey)
		_ = request.Context().Value(security.ClaimsContextKey)
	})
	request := benchmarkRequest()
	b.ReportAllocs()
	for b.Loop() {
		ctx := security.WithSecurityContext(request.Context(), "user", []string{"admin"}, map[string]any{"uuid": "user", "sub": "subject"}, "token")
		handler.ServeHTTP(httptest.NewRecorder(), request.WithContext(ctx))
	}
}

func BenchmarkOptimizedRequestContext(b *testing.B) {
	optimizationOnce = sync.Once{}
	optimizationOnce.Do(func() { optimizationEnabled = true })
	handler := RequestOptimizationMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		ctx := security.WithSecurityContext(request.Context(), "user", []string{"admin"}, map[string]any{"uuid": "user", "sub": "subject"}, "token")
		_ = ctx
		_, _ = RequestStateFromContext(request.Context())
	}))
	request := benchmarkRequest()
	b.ReportAllocs()
	for b.Loop() {
		handler.ServeHTTP(httptest.NewRecorder(), request)
	}
}

func benchmarkRequest() *http.Request {
	ctx := context.WithValue(context.Background(), security.UserContextKey, "user")
	ctx = context.WithValue(ctx, security.RolesContextKey, []string{"admin"})
	ctx = context.WithValue(ctx, security.ClaimsContextKey, map[string]any{"uuid": "user", "sub": "subject"})
	return httptest.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
}
