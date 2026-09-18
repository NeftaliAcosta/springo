package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/NeftaliAcosta/springo/framework/security"
)

func TestRequestOptimizationSanitizesPooledState(t *testing.T) {
	defer func() {
		optimizationOnce = sync.Once{}
		optimizationOnce.Do(func() { optimizationEnabled = false })
	}()
	optimizationOnce = sync.Once{}
	optimizationOnce.Do(func() { optimizationEnabled = true })
	var seen []*RequestState
	handler := RequestOptimizationMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		state, ok := RequestStateFromContext(request.Context())
		if !ok {
			t.Fatal("request state missing")
		}
		seen = append(seen, state)
	}))

	ctx := security.WithSecurityContext(context.Background(), "user-1", []string{"admin"}, map[string]any{"uuid": "user-1", "sub": "subject-1"}, "token")
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(ctx, http.MethodGet, "/", nil))
	if seen[0].User != "" || seen[0].Claims != nil || len(seen[0].Roles) != 0 {
		t.Fatalf("pooled state retained data: %#v", seen[0])
	}
}
