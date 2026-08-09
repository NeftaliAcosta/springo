package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"github.com/NeftaliAcosta/springo/framework/config"
	"github.com/NeftaliAcosta/springo/framework/errors"
	"net/http"
)

// CsrfProperties defines configuration for CSRF protection
type CsrfProperties struct {
	Enabled     bool     `yaml:"enabled"`
	CookieName  string   `yaml:"cookie-name"`
	HeaderName  string   `yaml:"header-name"`
	PublicPaths []string `yaml:"public-paths"`
}

func init() {
	config.RegisterProperties("spring.security.csrf", &CsrfProperties{
		Enabled:    false, // Disabled by default, opt-in for APIs
		CookieName: "XSRF-TOKEN",
		HeaderName: "X-XSRF-TOKEN",
	})
}

func generateCsrfToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("failed to generate secure random bytes for CSRF: " + err.Error())
	}
	return hex.EncodeToString(b)
}

func getCsrfProperties() *CsrfProperties {
	props := config.Get[CsrfProperties]()
	if props == nil {
		props = &CsrfProperties{
			Enabled:    false,
			CookieName: "XSRF-TOKEN",
			HeaderName: "X-XSRF-TOKEN",
		}
	}
	return props
}

func isSafeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}

func ensureCsrfCookie(w http.ResponseWriter, r *http.Request, props *CsrfProperties) {
	cookie, err := r.Cookie(props.CookieName)
	if err != nil || cookie.Value == "" {
		// Generate new token cookie
		token := generateCsrfToken()
		isSecure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
		http.SetCookie(w, &http.Cookie{
			Name:     props.CookieName,
			Value:    token,
			Path:     "/",
			HttpOnly: false, // Must be readable by client JS
			Secure:   isSecure,
			SameSite: http.SameSiteLaxMode,
		})
	}
}

func validateCsrfToken(r *http.Request, props *CsrfProperties) error {
	cookie, err := r.Cookie(props.CookieName)
	if err != nil || cookie.Value == "" {
		return errors.Forbidden("missing CSRF cookie token", "CSRF_COOKIE_MISSING")
	}

	headerToken := r.Header.Get(props.HeaderName)
	if headerToken == "" {
		return errors.Forbidden("missing CSRF header token", "CSRF_HEADER_MISSING")
	}

	// Use constant-time comparison to prevent timing attacks
	if len(cookie.Value) != len(headerToken) || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(headerToken)) != 1 {
		return errors.Forbidden("invalid CSRF token", "CSRF_TOKEN_INVALID")
	}
	return nil
}

// CsrfMiddleware enforces Double Submit Cookie CSRF protection
func CsrfMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		props := getCsrfProperties()

		if !props.Enabled || isPublicPath(r.URL.Path, props.PublicPaths) {
			next.ServeHTTP(w, r)
			return
		}

		if isSafeMethod(r.Method) {
			ensureCsrfCookie(w, r, props)
			next.ServeHTTP(w, r)
			return
		}

		if err := validateCsrfToken(r, props); err != nil {
			HandleError(w, r, err)
			return
		}

		next.ServeHTTP(w, r)
	})
}
