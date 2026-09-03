package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/NeftaliAcosta/springo/framework/config"
	"github.com/go-chi/chi/v5"
)

//go:embed dashboard/dashboard.html
var dashboardHTML string

//go:embed dashboard/dashboard.css
var dashboardCSS string

//go:embed dashboard/dashboard.js
var dashboardJS string

// BasicAuthProperties defines basic security settings.
type BasicAuthProperties struct {
	Name     string `yaml:"name"`
	Password string `yaml:"password"`
}

// Validate ensures required security properties are set for active profiles.
func (p *BasicAuthProperties) Validate() error {
	profile := strings.ToLower(strings.TrimSpace(os.Getenv("SPRINGO_PROFILES_ACTIVE")))
	if profile == "prod" || profile == "production" {
		if strings.TrimSpace(p.Password) == "" {
			return fmt.Errorf("spring.security.user.password must be explicitly set in production profile")
		}
	}
	return nil
}

// ExposureProperties controls exposed actuator endpoints.
type ExposureProperties struct {
	Include string `yaml:"include"`
}

// WebProperties wraps actuator web configuration.
type WebProperties struct {
	Exposure ExposureProperties `yaml:"exposure"`
}

// EndpointsProperties wraps actuator endpoint settings.
type EndpointsProperties struct {
	Web WebProperties `yaml:"web"`
}

// ManagementProperties contains management and monitoring configurations.
type ManagementProperties struct {
	Endpoints EndpointsProperties `yaml:"endpoints"`
}

func init() {
	config.RegisterProperties("spring.security.user", &BasicAuthProperties{
		Name:     "admin",
		Password: "", // Empty triggers secure random generation
	})

	config.RegisterProperties("management", &ManagementProperties{
		Endpoints: EndpointsProperties{
			Web: WebProperties{
				Exposure: ExposureProperties{
					Include: "*", // Expose everything by default
				},
			},
		},
	})
}

var (
	generatedPassword     string
	generatedPasswordOnce sync.Once
	dlqRetryCallback      func(ctx context.Context, eventName string, payload string) error
	dlqRetryCallbackMu    sync.RWMutex
)

// RegisterDlqRetryCallback registers a callback to re-dispatch events, avoiding circular imports.
func RegisterDlqRetryCallback(fn func(ctx context.Context, eventName string, payload string) error) {
	dlqRetryCallbackMu.Lock()
	defer dlqRetryCallbackMu.Unlock()
	dlqRetryCallback = fn
}

func getOrGenerateCredentials() (string, string) {
	props := config.Get[BasicAuthProperties]()
	if props == nil {
		props = &BasicAuthProperties{Name: "admin", Password: ""}
	}
	user := props.Name
	if user == "" {
		user = "admin"
	}
	pass := props.Password
	if pass == "" {
		generatedPasswordOnce.Do(func() {
			generatedPassword = generateRandomPassword(16)
		})
		pass = generatedPassword
	}
	return user, pass
}

func generateRandomPassword(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return string(b)
}

func isActuatorAuthenticated(r *http.Request) bool {
	expectedUser, expectedPass := getOrGenerateCredentials()
	user, pass, ok := r.BasicAuth()
	if !ok {
		return false
	}
	userMatch := subtle.ConstantTimeCompare([]byte(user), []byte(expectedUser)) == 1
	passMatch := subtle.ConstantTimeCompare([]byte(pass), []byte(expectedPass)) == 1
	return userMatch && passMatch
}

func isEndpointExposed(endpoint string) bool {
	props := config.Get[ManagementProperties]()
	include := "*"
	if props != nil && props.Endpoints.Web.Exposure.Include != "" {
		include = props.Endpoints.Web.Exposure.Include
	}
	if include == "*" {
		return true
	}
	parts := strings.Split(include, ",")
	for _, p := range parts {
		if strings.TrimSpace(p) == endpoint {
			return true
		}
	}
	return false
}

// ActuatorBasicAuthMiddleware protects sensitive actuator endpoints.
func ActuatorBasicAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only intercept /actuator paths
		if !isActuatorPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		// Public health check and info routes bypass authentication
		if r.URL.Path == "/actuator/health" || r.URL.Path == "/actuator/info" {
			next.ServeHTTP(w, r)
			return
		}

		if !isActuatorAuthenticated(r) {
			w.Header().Set("WWW-Authenticate", `Basic realm="SprinGo Actuator"`)
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("401 Unauthorized\n"))
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RegisterActuatorRoutes configures management endpoints on the chi router.
func RegisterActuatorRoutes(r chi.Router) {
	// Pre-generate credentials on startup to print password to console log immediately
	user, _ := getOrGenerateCredentials()
	if generatedPassword != "" {
		log.Printf("🔑 [Security] A temporary security password was generated for user '%s' "+
			"(set 'spring.security.user.password' to configure)", user)
	}

	// Add Basic Auth specifically for actuator endpoints
	r.Use(ActuatorBasicAuthMiddleware)

	r.Route("/actuator", func(r chi.Router) {
		r.Get("/health", handleActuatorHealth)
		r.Get("/info", handleActuatorInfo)
		r.Get("/dashboard", handleActuatorDashboard)
		r.Get("/css/dashboard.css", handleActuatorDashboardCSS)
		r.Get("/js/dashboard.js", handleActuatorDashboardJS)
		r.Get("/threaddump", handleActuatorThreadDump)
		r.Get("/loggers", handleActuatorLoggersGet)
		r.Post("/loggers", handleActuatorLoggersPost)
		r.Get("/beans", handleActuatorBeans)
		r.Get("/env", handleActuatorEnv)
		r.Get("/shedlock", handleActuatorShedlockGet)
		r.Post("/shedlock/trigger", handleActuatorShedlockTrigger)
		r.Get("/dlq", handleActuatorDlqGet)
		r.Post("/dlq/retry", handleActuatorDlqRetry)
		r.Post("/dlq/purge", handleActuatorDlqPurge)
	})
}
