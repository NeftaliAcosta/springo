package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeRequestURI(t *testing.T) {
	u1, _ := url.Parse("/api/v1/users?name=john&token=secret123&page=1")
	sanitized := sanitizeRequestURI(u1)
	assert.Contains(t, sanitized, "token=%2A%2A%2A%2A%2A%2A")
	assert.Contains(t, sanitized, "name=john")
	assert.Contains(t, sanitized, "page=1")

	u2, _ := url.Parse("/api/v1/orders")
	assert.Equal(t, "/api/v1/orders", sanitizeRequestURI(u2))
}

func TestStructuredLoggerMiddleware(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	loggedHandler := StructuredLoggerMiddleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/test/path?apiKey=supersecret", nil)
	rec := httptest.NewRecorder()

	loggedHandler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "ok", rec.Body.String())
}
