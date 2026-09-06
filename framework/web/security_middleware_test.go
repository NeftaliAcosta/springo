package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/NeftaliAcosta/springo/framework/config"
	"github.com/NeftaliAcosta/springo/framework/security"
)

// TestIsPublicPath verifies the correctness and safety of path matching rules.
func TestIsPublicPath(t *testing.T) {
	tests := []struct {
		name        string
		currentPath string
		publicPaths []string
		expected    bool
	}{
		{
			name:        "Exact Match Success",
			currentPath: "/api/v1/auth/login",
			publicPaths: []string{"/api/v1/auth/login"},
			expected:    true,
		},
		{
			name:        "Exact Match Success with Trailing Slash in Path",
			currentPath: "/api/v1/auth/login/",
			publicPaths: []string{"/api/v1/auth/login"},
			expected:    true,
		},
		{
			name:        "Exact Match Success with Trailing Slash in Rule",
			currentPath: "/api/v1/auth/login",
			publicPaths: []string{"/api/v1/auth/login/"},
			expected:    true,
		},
		{
			name:        "Subpath Match Success",
			currentPath: "/swagger/index.html",
			publicPaths: []string{"/swagger"},
			expected:    true,
		},
		{
			name:        "Subpath Match Success with Trailing Slash in Rule",
			currentPath: "/swagger/index.html",
			publicPaths: []string{"/swagger/"},
			expected:    true,
		},
		{
			name:        "Subpath Match Success Multi Level",
			currentPath: "/api/v1/actuator/health/detailed",
			publicPaths: []string{"/api/v1/actuator/health"},
			expected:    true,
		},
		{
			name:        "No Match Sibling Path Prefix",
			currentPath: "/api/v1/auth/login-admin",
			publicPaths: []string{"/api/v1/auth/login"},
			expected:    false,
		},
		{
			name:        "No Match Sibling Path Prefix Wildcard-Like",
			currentPath: "/swagger-admin/index.html",
			publicPaths: []string{"/swagger"},
			expected:    false,
		},
		{
			name:        "No Match Unrelated Path",
			currentPath: "/api/v1/users",
			publicPaths: []string{"/api/v1/auth/login", "/swagger"},
			expected:    false,
		},
		{
			name:        "Root Path Match",
			currentPath: "/",
			publicPaths: []string{"/"},
			expected:    true,
		},
		{
			name:        "Root Path No Match Subpath",
			currentPath: "/hello",
			publicPaths: []string{"/"},
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := isPublicPath(tt.currentPath, tt.publicPaths)
			if actual != tt.expected {
				t.Errorf("isPublicPath(%q, %v) = %v; want %v", tt.currentPath, tt.publicPaths, actual, tt.expected)
			}
		})
	}
}

// TestIsActuatorPath verifies actuator path matching.
func TestIsActuatorPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"Exact actuator root", "/actuator", true},
		{"Actuator root with trailing slash", "/actuator/", true},
		{"Actuator health subpath", "/actuator/health", true},
		{"Actuator loggers subpath", "/actuator/loggers", true},
		{"Sibling path prefix /actuator-publication", "/actuator-publication", false},
		{"Sibling path prefix /actuators", "/actuators", false},
		{"Sibling path prefix /actuator-data", "/actuator-data", false},
		{"Unrelated path /api/v1/users", "/api/v1/users", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := isActuatorPath(tt.path)
			if actual != tt.expected {
				t.Errorf("isActuatorPath(%q) = %v; want %v", tt.path, actual, tt.expected)
			}
		})
	}
}

func TestAuthMiddlewareValidTokenPasses(t *testing.T) {
	secret := "test-secret-32-bytes-long-for-valid-hs256"
	config.RegisterProperties("security.jwt", &security.JwtProperties{
		Secret:    secret,
		Algorithm: "HS256",
	})
	provider := security.NewJwtProvider(secret, 15)

	tokenStr, err := provider.GenerateTokenWithClaims("uuid-123", []string{"USER"}, map[string]interface{}{
		"preferred_username": "john_doe",
		"iss":                "https://auth.company.com/realms/main",
		"aud":                "billing-service",
	})
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	req, _ := http.NewRequest("GET", "/api/orders", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()

	var extractedUser string
	var extractedRoles []string

	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		extractedUser = security.GetUser(r.Context())
		extractedRoles = security.GetRoles(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	if extractedUser != "john_doe" && extractedUser != "uuid-123" {
		t.Fatalf("unexpected principal %q", extractedUser)
	}
	if len(extractedRoles) == 0 {
		t.Fatalf("expected roles in context")
	}
}

func TestAuthMiddlewareMissingTokenReturns401(t *testing.T) {
	req, _ := http.NewRequest("GET", "/api/protected", nil)
	rec := httptest.NewRecorder()

	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for missing token, got %d", rec.Code)
	}
}

func TestAuthMiddlewarePublicPathWildcardBypassesAuth(t *testing.T) {
	req, _ := http.NewRequest("GET", "/swagger/index.html", nil)
	rec := httptest.NewRecorder()

	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for public swagger path, got %d", rec.Code)
	}
}
