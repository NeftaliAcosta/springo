package web

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/NeftaliAcosta/springo/framework/ioc"
	"github.com/NeftaliAcosta/springo/framework/security"

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
	assert.Equal(t, "0", w.Header().Get("X-XSS-Protection"))
	assert.Equal(t, "strict-origin-when-cross-origin", w.Header().Get("Referrer-Policy"))
	assert.Equal(t, "camera=(), microphone=(), geolocation=(), payment=()", w.Header().Get("Permissions-Policy"))
	assert.Equal(t, "none", w.Header().Get("X-Permitted-Cross-Domain-Policies"))
	assert.Equal(t, "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:;", w.Header().Get("Content-Security-Policy"))
	assert.Empty(t, w.Header().Get("Strict-Transport-Security")) // Not HTTPS
}

func TestSecurityHeadersMiddleware_HTTPS(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/path", nil)
	req.TLS = &tls.ConnectionState{}
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
	t.Cleanup(func() {
		ioc.GetContainer().RegisterBean("CsrfProperties", &CsrfProperties{Enabled: false})
	})

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
	t.Cleanup(func() {
		ioc.GetContainer().RegisterBean("CsrfProperties", &CsrfProperties{Enabled: false})
	})

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
	t.Cleanup(func() {
		ioc.GetContainer().RegisterBean("CsrfProperties", &CsrfProperties{Enabled: false})
	})

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
	t.Cleanup(func() {
		ioc.GetContainer().RegisterBean("CsrfProperties", &CsrfProperties{Enabled: false})
	})

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
	t.Cleanup(func() {
		ioc.GetContainer().RegisterBean("CsrfProperties", &CsrfProperties{Enabled: false})
	})

	req := httptest.NewRequest(http.MethodPost, "/api/public", nil)
	w := httptest.NewRecorder()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	CsrfMiddleware(next).ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func boolPtr(b bool) *bool {
	return &b
}

func TestServerSecurityProperties_Defaults(t *testing.T) {
	props := &ServerSecurityProperties{}
	assert.False(t, props.IsCsrfEnabled())
	assert.True(t, props.IsSecurityHeadersEnabled())
	assert.True(t, props.IsCorsEnabled())
}

func TestServerSecurityProperties_OAuth2AutoBypass(t *testing.T) {
	enabled := true
	oauth2Props := &security.OAuth2ResourceServerProperties{
		Enabled: &enabled,
		Secret:  "test-secret-12345678901234567890",
	}
	ioc.GetContainer().RegisterBean("OAuth2ResourceServerProperties", oauth2Props)

	props := &ServerSecurityProperties{}
	assert.False(t, props.IsCsrfEnabled())

	propsExplicitTrue := &ServerSecurityProperties{CsrfEnabled: boolPtr(true)}
	assert.True(t, propsExplicitTrue.IsCsrfEnabled())

	ioc.GetContainer().RegisterBean("OAuth2ResourceServerProperties", (*security.OAuth2ResourceServerProperties)(nil))
}

func TestServerSecurityProperties_CsrfExplicitToggle(t *testing.T) {
	serverProps := &WebServerProperties{
		Security: ServerSecurityProperties{
			CsrfEnabled: boolPtr(true),
		},
	}
	ioc.GetContainer().RegisterBean("WebServerProperties", serverProps)

	csrfProps := &CsrfProperties{
		CookieName: "XSRF-TOKEN",
		HeaderName: "X-XSRF-TOKEN",
	}
	ioc.GetContainer().RegisterBean("CsrfProperties", csrfProps)

	req := httptest.NewRequest(http.MethodPost, "/api/resource", nil)
	w := httptest.NewRecorder()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	CsrfMiddleware(next).ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)

	serverProps.Security.CsrfEnabled = boolPtr(false)
	w2 := httptest.NewRecorder()
	CsrfMiddleware(next).ServeHTTP(w2, req)
	assert.Equal(t, http.StatusOK, w2.Code)
}

func TestServerSecurityProperties_SecurityHeadersToggle(t *testing.T) {
	serverProps := &WebServerProperties{
		Security: ServerSecurityProperties{
			SecurityHeadersEnabled: boolPtr(false),
		},
	}
	ioc.GetContainer().RegisterBean("WebServerProperties", serverProps)

	req := httptest.NewRequest(http.MethodGet, "/path", nil)
	w := httptest.NewRecorder()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	SecurityHeadersMiddleware(next).ServeHTTP(w, req)
	assert.Empty(t, w.Header().Get("X-Content-Type-Options"))
	assert.Empty(t, w.Header().Get("X-Frame-Options"))

	serverProps.Security.SecurityHeadersEnabled = boolPtr(true)
	w2 := httptest.NewRecorder()
	SecurityHeadersMiddleware(next).ServeHTTP(w2, req)
	assert.Equal(t, "nosniff", w2.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", w2.Header().Get("X-Frame-Options"))
}

func TestServerSecurityProperties_CorsToggle(t *testing.T) {
	corsProps := &CorsProperties{
		AllowedOrigins: []string{"http://localhost:3000"},
		AllowedMethods: []string{"GET", "POST"},
	}
	ioc.GetContainer().RegisterBean("CorsProperties", corsProps)

	serverProps := &WebServerProperties{
		Security: ServerSecurityProperties{
			CorsEnabled: boolPtr(false),
		},
	}
	ioc.GetContainer().RegisterBean("WebServerProperties", serverProps)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	CorsMiddleware(next).ServeHTTP(w, req)
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))

	serverProps.Security.CorsEnabled = boolPtr(true)
	w2 := httptest.NewRecorder()
	CorsMiddleware(next).ServeHTTP(w2, req)
	assert.Equal(t, "http://localhost:3000", w2.Header().Get("Access-Control-Allow-Origin"))
}

func TestWebServerProperties_Validate_IncludesSecurity(t *testing.T) {
	props := &WebServerProperties{
		API: APIProperties{BasePath: "/api/v1"},
		Multipart: MultipartProperties{
			Enabled:         true,
			MaxFileSize:     1024,
			MaxRequestSize:  2048,
			MemoryThreshold: 512,
		},
		Security: ServerSecurityProperties{
			Headers: SecurityHeadersProperties{
				HSTS: HSTSProperties{MaxAge: 600},
			},
		},
	}
	assert.NoError(t, props.Validate())

	invalidProps := &WebServerProperties{
		API: APIProperties{BasePath: "/api/v1"},
		Security: ServerSecurityProperties{
			Headers: SecurityHeadersProperties{
				HSTS: HSTSProperties{MaxAge: -1},
			},
		},
	}
	assert.Error(t, invalidProps.Validate())
}

func TestSecurityHeadersMiddleware_CustomHeaders(t *testing.T) {
	includeSub := false
	serverProps := &WebServerProperties{
		Security: ServerSecurityProperties{
			Headers: SecurityHeadersProperties{
				FrameOptions:          "SAMEORIGIN",
				ContentSecurityPolicy: "default-src 'self' https://cdn.example.com",
				HSTS: HSTSProperties{
					MaxAge:            7200,
					IncludeSubdomains: &includeSub,
					Preload:           true,
				},
			},
		},
	}
	ioc.GetContainer().RegisterBean("WebServerProperties", serverProps)
	t.Cleanup(func() {
		ioc.GetContainer().RegisterBean("WebServerProperties", (*WebServerProperties)(nil))
	})

	req := httptest.NewRequest(http.MethodGet, "/path", nil)
	req.TLS = &tls.ConnectionState{}
	w := httptest.NewRecorder()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	SecurityHeadersMiddleware(next).ServeHTTP(w, req)

	assert.Equal(t, "SAMEORIGIN", w.Header().Get("X-Frame-Options"))
	assert.Equal(t, "default-src 'self' https://cdn.example.com", w.Header().Get("Content-Security-Policy"))
	assert.Equal(t, "max-age=7200; preload", w.Header().Get("Strict-Transport-Security"))
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
}


