package web

import (
	"testing"
)

// TestIsPublicPath verifies the correctness and safety of path matching rules.
// It ensures that exact path matching and directory/subpath boundaries are respected,
// preventing security bypasses where a sibling endpoint shares the same prefix.
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
			name:        "No Match Sibling Path Prefix (Security Bypass Mitigation)",
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
