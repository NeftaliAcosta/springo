package web

import (
	"net/http"
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
