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

// validateAndPrepareConfig performs enterprise security checks and compiles patterns.
func validateAndPrepareConfig(props *CorsProperties) {
	cacheOnce.Do(func() {
		if props == nil {
			return
		}

		if err := validateCredentialsOrigin(props); err != nil {
			configError = err
			log.Printf("❌ [CORS ERROR] %v", configError)
			return
		}

		compileOriginPatterns(props.AllowedOriginPatterns)
	})
}

// validateCredentialsOrigin checks that wildcard origins are not used when allow-credentials is true.
func validateCredentialsOrigin(props *CorsProperties) error {
	if !props.AllowCredentials {
		return nil
	}

	for _, o := range props.AllowedOrigins {
		if o == "*" {
			return fmt.Errorf("insecure CORS configuration: 'allow-credentials' is true but 'allowed-origins' contains '*'")
		}
	}
	return nil
}

// compileOriginPatterns converts wildcard patterns (e.g. *.example.com) to regex and caches them.
func compileOriginPatterns(patterns []string) {
	for _, pattern := range patterns {
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
}

// isCorsDisabled checks if CORS processing should be skipped based on configuration.
func isCorsDisabled(props *CorsProperties) bool {
	if props == nil {
		return true
	}
	serverProps := config.Get[WebServerProperties]()
	return serverProps != nil && !serverProps.Security.IsCorsEnabled()
}

// isExactOriginAllowed checks if the origin matches any explicitly allowed origin.
func isExactOriginAllowed(allowedOrigins []string, allowCredentials bool, origin string) bool {
	for _, o := range allowedOrigins {
		if o == "*" && !allowCredentials {
			return true
		}
		if o == origin {
			return true
		}
	}
	return false
}

// isPatternOriginAllowed checks if the origin matches any compiled regex pattern.
func isPatternOriginAllowed(origin string) bool {
	for _, re := range patternCache {
		if re.MatchString(origin) {
			return true
		}
	}
	return false
}

// isOriginAllowed determines if the requested origin is permitted by exact match or pattern.
func isOriginAllowed(props *CorsProperties, origin string) bool {
	if isExactOriginAllowed(props.AllowedOrigins, props.AllowCredentials, origin) {
		return true
	}
	return isPatternOriginAllowed(origin)
}

// applyOriginHeaders sets Access-Control-Allow-Origin and Vary headers.
func applyOriginHeaders(w http.ResponseWriter, props *CorsProperties, origin string) {
	if props.AllowCredentials || !contains(props.AllowedOrigins, "*") {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
}

// applyCorsHeaders sets standard CORS response headers for allowed origins.
func applyCorsHeaders(w http.ResponseWriter, props *CorsProperties, origin string) {
	applyOriginHeaders(w, props, origin)

	if props.AllowCredentials {
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}
	if len(props.ExposedHeaders) > 0 {
		w.Header().Set("Access-Control-Expose-Headers", strings.Join(props.ExposedHeaders, ", "))
	}
}

// applyPreflightHeaders sets preflight CORS headers for OPTIONS requests.
func applyPreflightHeaders(w http.ResponseWriter, props *CorsProperties) {
	if len(props.AllowedMethods) > 0 {
		w.Header().Set("Access-Control-Allow-Methods", strings.Join(props.AllowedMethods, ", "))
	}
	if len(props.AllowedHeaders) > 0 {
		w.Header().Set("Access-Control-Allow-Headers", strings.Join(props.AllowedHeaders, ", "))
	}
	if props.MaxAge > 0 {
		w.Header().Set("Access-Control-Max-Age", fmt.Sprintf("%d", props.MaxAge))
	}
}

// handlePreflight processes preflight OPTIONS requests and returns true if handled.
func handlePreflight(w http.ResponseWriter, r *http.Request, props *CorsProperties) bool {
	if r.Method != http.MethodOptions {
		return false
	}
	applyPreflightHeaders(w, props)
	w.WriteHeader(http.StatusNoContent)
	return true
}

// CorsMiddleware handles Cross-Origin Resource Sharing based on YAML configuration.
func CorsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		props := config.Get[CorsProperties]()
		if isCorsDisabled(props) {
			next.ServeHTTP(w, r)
			return
		}

		validateAndPrepareConfig(props)
		if configError != nil {
			http.Error(w, "Internal Server Error: Invalid CORS Configuration", http.StatusInternalServerError)
			return
		}

		origin := r.Header.Get("Origin")
		if origin == "" || !isOriginAllowed(props, origin) {
			next.ServeHTTP(w, r)
			return
		}

		applyCorsHeaders(w, props, origin)
		if handlePreflight(w, r, props) {
			return
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
