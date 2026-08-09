package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/NeftaliAcosta/springo/framework/ioc"

	"github.com/stretchr/testify/assert"
)

func TestSecurityHeadersMiddleware(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/path", nil)
	w := httptest.NewRecorder()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	SecurityHeadersMiddleware(next).ServeHTTP(w, req)

	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
	assert.Equal(t, "1; mode=block", w.Header().Get("X-XSS-Protection"))
	assert.Equal(t, "no-referrer-when-downgrade", w.Header().Get("Referrer-Policy"))
	assert.Empty(t, w.Header().Get("Strict-Transport-Security")) // Not HTTPS
}

func TestSecurityHeadersMiddleware_HTTPS(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/path", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	SecurityHeadersMiddleware(next).ServeHTTP(w, req)
	assert.Equal(t, "max-age=31536000; includeSubDomains", w.Header().Get("Strict-Transport-Security"))
}

func TestCsrfMiddleware_DisabledByDefault(t *testing.T) {
	// Reset/register disabled CSRF props
	props := &CsrfProperties{Enabled: false}
	ioc.GetContainer().RegisterBean("CsrfProperties", props)

	req := httptest.NewRequest(http.MethodPost, "/api/create", nil)
	w := httptest.NewRecorder()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	CsrfMiddleware(next).ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestCsrfMiddleware_SafeMethodSetsCookie(t *testing.T) {
	props := &CsrfProperties{
		Enabled:    true,
		CookieName: "XSRF-TOKEN",
		HeaderName: "X-XSRF-TOKEN",
	}
	ioc.GetContainer().RegisterBean("CsrfProperties", props)

	req := httptest.NewRequest(http.MethodGet, "/api/resource", nil)
	w := httptest.NewRecorder()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	CsrfMiddleware(next).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	cookies := w.Result().Cookies()
	assert.NotEmpty(t, cookies)

	var csrfCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "XSRF-TOKEN" {
			csrfCookie = c
			break
		}
	}
	assert.NotNil(t, csrfCookie)
	assert.NotEmpty(t, csrfCookie.Value)
	assert.False(t, csrfCookie.HttpOnly)
}

func TestCsrfMiddleware_UnsafeMethodMissingToken(t *testing.T) {
	props := &CsrfProperties{
		Enabled:    true,
		CookieName: "XSRF-TOKEN",
		HeaderName: "X-XSRF-TOKEN",
	}
	ioc.GetContainer().RegisterBean("CsrfProperties", props)

	req := httptest.NewRequest(http.MethodPost, "/api/resource", nil)
	w := httptest.NewRecorder()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	CsrfMiddleware(next).ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestCsrfMiddleware_UnsafeMethodMatchingToken(t *testing.T) {
	props := &CsrfProperties{
		Enabled:    true,
		CookieName: "XSRF-TOKEN",
		HeaderName: "X-XSRF-TOKEN",
	}
	ioc.GetContainer().RegisterBean("CsrfProperties", props)

	tokenVal := "secure-csrf-token-1234"

	req := httptest.NewRequest(http.MethodPost, "/api/resource", nil)
	req.AddCookie(&http.Cookie{Name: "XSRF-TOKEN", Value: tokenVal})
	req.Header.Set("X-XSRF-TOKEN", tokenVal)

	w := httptest.NewRecorder()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	CsrfMiddleware(next).ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestCsrfMiddleware_UnsafeMethodMismatchToken(t *testing.T) {
	props := &CsrfProperties{
		Enabled:    true,
		CookieName: "XSRF-TOKEN",
		HeaderName: "X-XSRF-TOKEN",
	}
	ioc.GetContainer().RegisterBean("CsrfProperties", props)

	req := httptest.NewRequest(http.MethodPost, "/api/resource", nil)
	req.AddCookie(&http.Cookie{Name: "XSRF-TOKEN", Value: "token-a"})
	req.Header.Set("X-XSRF-TOKEN", "token-b")

	w := httptest.NewRecorder()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	CsrfMiddleware(next).ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestCsrfMiddleware_PublicPathBypass(t *testing.T) {
	props := &CsrfProperties{
		Enabled:     true,
		CookieName:  "XSRF-TOKEN",
		HeaderName:  "X-XSRF-TOKEN",
		PublicPaths: []string{"/api/public"},
	}
	ioc.GetContainer().RegisterBean("CsrfProperties", props)

	req := httptest.NewRequest(http.MethodPost, "/api/public", nil)
	w := httptest.NewRecorder()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	CsrfMiddleware(next).ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}
