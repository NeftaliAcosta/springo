package web

import (
	"fmt"
	"github.com/NeftaliAcosta/springo/framework/config"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
)

// CorsProperties defines the CORS configuration in application.yaml
type CorsProperties struct {
	AllowedOrigins        []string `yaml:"allowed-origins"`
	AllowedOriginPatterns []string `yaml:"allowed-origin-patterns"`
	AllowedMethods        []string `yaml:"allowed-methods"`
	AllowedHeaders        []string `yaml:"allowed-headers"`
	ExposedHeaders        []string `yaml:"exposed-headers"`
	AllowCredentials      bool     `yaml:"allow-credentials"`
	MaxAge                int      `yaml:"max-age"`
}

var (
	patternCache [](*regexp.Regexp)
	cacheOnce    sync.Once
	configError  error
)

func init() {
	// Automatically register CORS properties under "server.cors" prefix
	config.RegisterProperties("server.cors", &CorsProperties{})
}

// validateAndPrepareConfig performs enterprise security checks and compiles patterns
func validateAndPrepareConfig(props *CorsProperties) {
	cacheOnce.Do(func() {
		if props == nil {
			return
		}

		// Security Check: If credentials are allowed, origins cannot be "*"
		if props.AllowCredentials {
			for _, o := range props.AllowedOrigins {
				if o == "*" {
					configError = fmt.Errorf("insecure CORS configuration: 'allow-credentials' is true but 'allowed-origins' contains '*'")
					log.Printf("❌ [CORS ERROR] %v", configError)
					return
				}
			}
		}

		// Pre-compile origin patterns for performance
		for _, pattern := range props.AllowedOriginPatterns {
			// Convert wildcard patterns (e.g. *.example.com) to valid regex
			regexStr := strings.ReplaceAll(pattern, ".", "\\.")
			regexStr = strings.ReplaceAll(regexStr, "*", ".*")
			regexStr = "^" + regexStr + "$"

			re, err := regexp.Compile(regexStr)
			if err != nil {
				log.Printf("⚠️ [CORS] Invalid origin pattern ignored: %s", pattern)
				continue
			}
			patternCache = append(patternCache, re)
		}
	})
}

// CorsMiddleware handles Cross-Origin Resource Sharing based on YAML configuration
func CorsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		props := config.Get[CorsProperties]()
		if props == nil {
			next.ServeHTTP(w, r)
			return
		}

		// Ensure config is validated and patterns are compiled (lazy-init)
		validateAndPrepareConfig(props)
		if configError != nil {
			http.Error(w, "Internal Server Error: Invalid CORS Configuration", http.StatusInternalServerError)
			return
		}

		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}

		isAllowed := false

		// 1. Check exact origins
		for _, o := range props.AllowedOrigins {
			if o == "*" {
				if !props.AllowCredentials {
					isAllowed = true
					break
				}
			} else if o == origin {
				isAllowed = true
				break
			}
		}

		// 2. Check origin patterns if not already allowed
		if !isAllowed && len(patternCache) > 0 {
			for _, re := range patternCache {
				if re.MatchString(origin) {
					isAllowed = true
					break
				}
			}
		}

		if isAllowed {
			// Set Origin header
			if props.AllowCredentials || !contains(props.AllowedOrigins, "*") {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin") // Essential for caching when origin is dynamic
			} else {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			}

			if props.AllowCredentials {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			if len(props.ExposedHeaders) > 0 {
				w.Header().Set("Access-Control-Expose-Headers", strings.Join(props.ExposedHeaders, ", "))
			}

			// Handle Preflight (OPTIONS) requests
			if r.Method == http.MethodOptions {
				if len(props.AllowedMethods) > 0 {
					w.Header().Set("Access-Control-Allow-Methods", strings.Join(props.AllowedMethods, ", "))
				}
				if len(props.AllowedHeaders) > 0 {
					w.Header().Set("Access-Control-Allow-Headers", strings.Join(props.AllowedHeaders, ", "))
				}
				if props.MaxAge > 0 {
					w.Header().Set("Access-Control-Max-Age", fmt.Sprintf("%d", props.MaxAge))
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func contains(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}
