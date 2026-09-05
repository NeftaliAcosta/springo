package web

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/NeftaliAcosta/springo/framework/ioc"
	"github.com/stretchr/testify/assert"
)

func resetCorsCache() {
	patternCache = nil
	cacheOnce = sync.Once{}
	configError = nil
}

func TestCorsMiddleware_NoOriginHeader(t *testing.T) {
	resetCorsCache()
	props := &CorsProperties{
		AllowedOrigins: []string{"http://localhost:3000"},
	}
	ioc.GetContainer().RegisterBean("CorsProperties", props)

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	w := httptest.NewRecorder()

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	CorsMiddleware(next).ServeHTTP(w, req)

	assert.True(t, called)
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
}

func TestCorsMiddleware_OriginNotAllowed(t *testing.T) {
	resetCorsCache()
	props := &CorsProperties{
		AllowedOrigins: []string{"http://localhost:3000"},
	}
	ioc.GetContainer().RegisterBean("CorsProperties", props)

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	req.Header.Set("Origin", "http://evil.com")
	w := httptest.NewRecorder()

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	CorsMiddleware(next).ServeHTTP(w, req)

	assert.True(t, called)
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
}

func TestCorsMiddleware_WildcardOriginWithoutCredentials(t *testing.T) {
	resetCorsCache()
	props := &CorsProperties{
		AllowedOrigins: []string{"*"},
	}
	ioc.GetContainer().RegisterBean("CorsProperties", props)

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	req.Header.Set("Origin", "http://any-domain.com")
	w := httptest.NewRecorder()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	CorsMiddleware(next).ServeHTTP(w, req)

	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Empty(t, w.Header().Get("Vary"))
}

func TestCorsMiddleware_AllowedOriginWithCredentials(t *testing.T) {
	resetCorsCache()
	props := &CorsProperties{
		AllowedOrigins:   []string{"http://localhost:3000"},
		AllowCredentials: true,
		ExposedHeaders:   []string{"X-Custom-Header"},
	}
	ioc.GetContainer().RegisterBean("CorsProperties", props)

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	CorsMiddleware(next).ServeHTTP(w, req)

	assert.Equal(t, "http://localhost:3000", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "Origin", w.Header().Get("Vary"))
	assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
	assert.Equal(t, "X-Custom-Header", w.Header().Get("Access-Control-Expose-Headers"))
}

func TestCorsMiddleware_AllowedOriginPattern(t *testing.T) {
	resetCorsCache()
	props := &CorsProperties{
		AllowedOriginPatterns: []string{"https://*.example.com"},
		AllowedOrigins:        []string{"https://app.other.com"},
	}
	ioc.GetContainer().RegisterBean("CorsProperties", props)

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	req.Header.Set("Origin", "https://sub.example.com")
	w := httptest.NewRecorder()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	CorsMiddleware(next).ServeHTTP(w, req)

	assert.Equal(t, "https://sub.example.com", w.Header().Get("Access-Control-Allow-Origin"))
}

func TestCorsMiddleware_PreflightOptions(t *testing.T) {
	resetCorsCache()
	props := &CorsProperties{
		AllowedOrigins: []string{"http://localhost:3000"},
		AllowedMethods: []string{"GET", "POST", "PUT"},
		AllowedHeaders: []string{"Content-Type", "Authorization"},
		MaxAge:         3600,
	}
	ioc.GetContainer().RegisterBean("CorsProperties", props)

	req := httptest.NewRequest(http.MethodOptions, "/api/data", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	CorsMiddleware(next).ServeHTTP(w, req)

	assert.False(t, called)
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "GET, POST, PUT", w.Header().Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "Content-Type, Authorization", w.Header().Get("Access-Control-Allow-Headers"))
	assert.Equal(t, "3600", w.Header().Get("Access-Control-Max-Age"))
}

func TestCorsMiddleware_InvalidConfigError(t *testing.T) {
	resetCorsCache()
	props := &CorsProperties{
		AllowedOrigins:   []string{"*"},
		AllowCredentials: true,
	}
	ioc.GetContainer().RegisterBean("CorsProperties", props)

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	CorsMiddleware(next).ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestCorsMiddleware_NilProperties(t *testing.T) {
	resetCorsCache()
	ioc.GetContainer().RegisterBean("CorsProperties", (*CorsProperties)(nil))

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	CorsMiddleware(next).ServeHTTP(w, req)

	assert.True(t, called)
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
}
