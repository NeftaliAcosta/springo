package web

import (
	"fmt"
	"github.com/NeftaliAcosta/springo/framework/config"
	"log"
	"net"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// DefaultMiddlewareHook allows adding custom middlewares during router creation
type DefaultMiddlewareHook func(r chi.Router)

// CreateDefaultRouter initializes a chi router with all framework standard middlewares and registered routes
func CreateDefaultRouter(customMiddlewares ...DefaultMiddlewareHook) chi.Router {
	r := chi.NewRouter()

	// 1. SprinGo Framework Middlewares (CORS, Tracing, and RequestScope first)
	r.Use(CorsMiddleware)
	r.Use(SecurityHeadersMiddleware)
	r.Use(CsrfMiddleware)
	r.Use(TracingMiddleware)
	r.Use(RequestScopeMiddleware)

	defaultLocale := "en"
	if i18nProps := config.Get[I18nProperties](); i18nProps != nil && i18nProps.DefaultLocale != "" {
		defaultLocale = i18nProps.DefaultLocale
	}
	r.Use(I18nMiddleware(defaultLocale))

	// 2. Standard Chi Middlewares
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(InterceptorMiddleware)

	// 3. Custom/External Middlewares (like Security)
	for _, hook := range customMiddlewares {
		hook(r)
	}

	// 3.5 Register Actuator Core routes
	RegisterActuatorRoutes(r)

	// 4. Register all auto-discovered routes
	RegisterAllRoutes(r)

	return r
}

type WebServerProperties struct {
	Port              int                 `yaml:"port"`
	API               APIProperties       `yaml:"api"`
	Multipart         MultipartProperties `yaml:"multipart"`
	ReadHeaderTimeout time.Duration       `yaml:"read-header-timeout"`
	ReadTimeout       time.Duration       `yaml:"read-timeout"`
	WriteTimeout      time.Duration       `yaml:"write-timeout"`
	IdleTimeout       time.Duration       `yaml:"idle-timeout"`
}

// MultipartProperties controls multipart/form-data request processing.
// File contents above MemoryThreshold are transparently spooled to temporary
// files by net/http and removed automatically after the controller returns.
type MultipartProperties struct {
	Enabled         bool  `yaml:"enabled"`
	MaxFileSize     int64 `yaml:"max-file-size"`
	MaxRequestSize  int64 `yaml:"max-request-size"`
	MemoryThreshold int64 `yaml:"memory-threshold"`
}

// APIProperties controls the common prefix applied to application routes.
type APIProperties struct {
	BasePath string `yaml:"base-path"`
}

// Validate normalizes and validates the application route prefix.
func (p *WebServerProperties) Validate() error {
	basePath := strings.TrimSpace(p.API.BasePath)
	if basePath == "" {
		basePath = DefaultAPIBasePath
	}
	if !strings.HasPrefix(basePath, "/") {
		return fmt.Errorf("server.api.base-path must start with '/'")
	}
	if strings.ContainsAny(basePath, "?#") {
		return fmt.Errorf("server.api.base-path cannot contain query or fragment characters")
	}
	p.API.BasePath = path.Clean(basePath)
	if !p.Multipart.Enabled && p.Multipart.MaxFileSize == 0 && p.Multipart.MaxRequestSize == 0 && p.Multipart.MemoryThreshold == 0 {
		return nil
	}
	if p.Multipart.MaxFileSize <= 0 {
		return fmt.Errorf("server.multipart.max-file-size must be greater than zero")
	}
	if p.Multipart.MaxRequestSize <= 0 {
		return fmt.Errorf("server.multipart.max-request-size must be greater than zero")
	}
	if p.Multipart.MaxFileSize > p.Multipart.MaxRequestSize {
		return fmt.Errorf("server.multipart.max-file-size cannot exceed max-request-size")
	}
	if p.Multipart.MemoryThreshold <= 0 || p.Multipart.MemoryThreshold > p.Multipart.MaxRequestSize {
		return fmt.Errorf("server.multipart.memory-threshold must be greater than zero and cannot exceed max-request-size")
	}
	return nil
}

func init() {
	config.RegisterProperties("server", &WebServerProperties{
		API: APIProperties{BasePath: DefaultAPIBasePath},
		Multipart: MultipartProperties{
			Enabled: true, MaxFileSize: 100 << 20, MaxRequestSize: 110 << 20, MemoryThreshold: 8 << 20,
		},
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	})
}

// BuildServer constructs an http.Server and binds a dynamic TCP listener securely
func BuildServer(startPort int, r chi.Router) (*http.Server, net.Listener, error) {
	port := startPort
	var ln net.Listener
	var err error

	for {
		addr := fmt.Sprintf(":%d", port)
		ln, err = net.Listen("tcp", addr)
		if err == nil {
			break
		}
		log.Printf("⚠️  Port %d occupied, trying %d...", port, port+1)
		port++
		if port > startPort+100 {
			return nil, nil, fmt.Errorf("could not find an available port in range %d-%d", startPort, startPort+100)
		}
	}

	props := config.Get[WebServerProperties]()
	if props == nil {
		props = &WebServerProperties{
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       60 * time.Second,
		}
	}

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           r,
		ReadHeaderTimeout: props.ReadHeaderTimeout,
		ReadTimeout:       props.ReadTimeout,
		WriteTimeout:      props.WriteTimeout,
		IdleTimeout:       props.IdleTimeout,
	}

	return server, ln, nil
}

// StartServerWithDynamicPort starts the server on the given port or the next available one
func StartServerWithDynamicPort(startPort int, r chi.Router) error {
	server, ln, err := BuildServer(startPort, r)
	if err != nil {
		return err
	}
	log.Printf("🚀 SprinGo Server running on http://localhost%s", server.Addr)
	return server.Serve(ln)
}
