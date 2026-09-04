package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestRegisterRoutesUsesConfiguredBasePath(t *testing.T) {
	router := chi.NewRouter()
	registerRoutes(router, "/platform/v2", []RegistrationHook{
		func(r chi.Router) { r.Get("/users", func(http.ResponseWriter, *http.Request) {}) },
	})

	routes, err := InspectRoutes(router)
	if err != nil {
		t.Fatalf("inspect routes: %v", err)
	}
	if len(routes) != 1 || routes[0].Pattern != "/platform/v2/users" {
		t.Fatalf("expected /platform/v2/users, got %#v", routes)
	}
}

func TestRegisterRoutesSupportsRootBasePath(t *testing.T) {
	router := chi.NewRouter()
	registerRoutes(router, "/", []RegistrationHook{
		func(r chi.Router) { r.Get("/users", func(http.ResponseWriter, *http.Request) {}) },
	})

	routes, err := InspectRoutes(router)
	if err != nil {
		t.Fatalf("inspect routes: %v", err)
	}
	if len(routes) != 1 || routes[0].Pattern != "/users" {
		t.Fatalf("expected /users, got %#v", routes)
	}
}

func TestWebServerPropertiesValidateBasePath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
		err  bool
	}{
		{name: "default", want: DefaultAPIBasePath},
		{name: "trailing slash", in: "/platform/v2/", want: "/platform/v2"},
		{name: "root", in: "/", want: "/"},
		{name: "missing leading slash", in: "platform/v2", err: true},
		{name: "query", in: "/platform?version=2", err: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			props := &WebServerProperties{API: APIProperties{BasePath: tt.in}}
			err := props.Validate()
			if (err != nil) != tt.err {
				t.Fatalf("Validate() error = %v, want error %v", err, tt.err)
			}
			if !tt.err && props.API.BasePath != tt.want {
				t.Fatalf("base path = %q, want %q", props.API.BasePath, tt.want)
			}
		})
	}
}

func TestRouteGroupIsolation(t *testing.T) {
	router := chi.NewRouter()

	mwV1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-V1-Scoped", "active")
			next.ServeHTTP(w, r)
		})
	}

	mwV2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-V2-Scoped", "active")
			next.ServeHTTP(w, r)
		})
	}

	RouteGroupOn(router, "/v1", []func(http.Handler) http.Handler{mwV1}, func(r chi.Router) {
		r.Get("/items", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	})

	RouteGroupOn(router, "/v2", []func(http.Handler) http.Handler{mwV2}, func(r chi.Router) {
		r.Get("/items", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	})

	// Test V1 request: must have X-V1-Scoped and NOT X-V2-Scoped.
	req1 := httptest.NewRequest(http.MethodGet, "/v1/items", nil)
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec1.Code)
	}
	if rec1.Header().Get("X-V1-Scoped") != "active" {
		t.Fatalf("expected X-V1-Scoped on /v1/items")
	}
	if rec1.Header().Get("X-V2-Scoped") != "" {
		t.Fatalf("did not expect X-V2-Scoped on /v1/items")
	}

	// Test V2 request: must have X-V2-Scoped and NOT X-V1-Scoped.
	req2 := httptest.NewRequest(http.MethodGet, "/v2/items", nil)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec2.Code)
	}
	if rec2.Header().Get("X-V2-Scoped") != "active" {
		t.Fatalf("expected X-V2-Scoped on /v2/items")
	}
	if rec2.Header().Get("X-V1-Scoped") != "" {
		t.Fatalf("did not expect X-V1-Scoped on /v2/items")
	}
}

func TestRouteGroupOnNilSafety(t *testing.T) {
	// Should not panic on nil router or nil register func.
	RouteGroupOn(nil, "/v1", nil, nil)
	router := chi.NewRouter()
	RouteGroupOn(router, "/v1", nil, nil)
}

