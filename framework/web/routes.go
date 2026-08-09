package web

import (
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
)

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
	mainRouter.Route("/api/v1", func(r chi.Router) {
		for _, hook := range hooks {
			hook(r)
		}
	})
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
