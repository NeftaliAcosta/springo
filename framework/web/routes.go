package web

import (
	"net/http"
	"sync"

	"github.com/NeftaliAcosta/springo/framework/config"
	"github.com/go-chi/chi/v5"
)

// DefaultAPIBasePath preserves SprinGo's original application route prefix.
const DefaultAPIBasePath = "/api/v1"

// RegistrationHook is a function that takes a router to register endpoints
type RegistrationHook func(r chi.Router)

var (
	hooksMu           sync.RWMutex
	registrationHooks []RegistrationHook
)

// Register adds a new hook to the list (used by Controllers)
func Register(hook RegistrationHook) {
	hooksMu.Lock()
	defer hooksMu.Unlock()
	registrationHooks = append(registrationHooks, hook)
}

// getHooksSnapshot returns a thread-safe copy of the registered hooks
func getHooksSnapshot() []RegistrationHook {
	hooksMu.RLock()
	defer hooksMu.RUnlock()
	snapshot := make([]RegistrationHook, len(registrationHooks))
	copy(snapshot, registrationHooks)
	return snapshot
}

// RegisterAllRoutes applies all registered hooks to the main router
func RegisterAllRoutes(mainRouter chi.Router) {
	hooks := getHooksSnapshot()
	registerRoutes(mainRouter, APIBasePath(), hooks)
}

// APIBasePath returns the normalized prefix configured at server.api.base-path.
func APIBasePath() string {
	props := config.Get[WebServerProperties]()
	if props == nil || props.API.BasePath == "" {
		return DefaultAPIBasePath
	}
	return props.API.BasePath
}

func registerRoutes(mainRouter chi.Router, basePath string, hooks []RegistrationHook) {
	apply := func(r chi.Router) {
		for _, hook := range hooks {
			hook(r)
		}
	}
	if basePath == "/" {
		apply(mainRouter)
		return
	}
	mainRouter.Route(basePath, apply)
}

// RouteInfo holds metadata about a registered route endpoint
type RouteInfo struct {
	Method      string
	Pattern     string
	HandlerName string
}

// InspectRoutes walks a chi router to return all route metadata without triggering side effects
func InspectRoutes(r chi.Router) ([]RouteInfo, error) {
	var routes []RouteInfo
	err := chi.Walk(r, func(method string, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		routes = append(routes, RouteInfo{
			Method:  method,
			Pattern: route,
		})
		return nil
	})
	return routes, err
}

// GetRegisteredRoutes builds an in-memory router with registered controller hooks to inspect route metadata
func GetRegisteredRoutes() []RouteInfo {
	r := chi.NewRouter()
	RegisterAllRoutes(r)
	routes, _ := InspectRoutes(r)
	return routes
}
