package web

import (
	"context"
	"fmt"
	"github.com/NeftaliAcosta/springo/framework/config"
	"github.com/NeftaliAcosta/springo/framework/errors"
	"github.com/NeftaliAcosta/springo/framework/security"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// isActuatorPath checks if the current request path matches the actuator base endpoint or any of its subpaths safely.
func isActuatorPath(path string) bool {
	p := strings.TrimSuffix(path, "/")
	return p == "/actuator" || strings.HasPrefix(path, "/actuator/")
}

// AuthMiddleware creates a middleware that validates JWT tokens based on framework properties
func AuthMiddleware(next http.Handler) http.Handler {
	props := config.Get[security.JwtProperties]()
	if props == nil {
		props = &security.JwtProperties{Secret: "default-secret", PublicPaths: []string{"/swagger"}}
	}

	provider := security.NewJwtProvider(props.Secret, props.Expiration).
		WithAsymmetricConfig(props.JwksURL, props.PublicKey, props.Algorithm)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isActuatorPath(r.URL.Path) || isPublicPath(r.URL.Path, props.PublicPaths) {
			next.ServeHTTP(w, r)
			return
		}

		tokenStr, err := extractToken(r)
		if err != nil {
			HandleError(w, r, errors.Unauthorized(err.Error(), "AUTH_HEADER_ERROR"))
			return
		}

		token, err := provider.ValidateToken(tokenStr)
		if err != nil || !token.Valid {
			HandleError(w, r, errors.Unauthorized("invalid or expired token", "AUTH_INVALID_TOKEN"))
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok || !token.Valid {
			HandleError(w, r, errors.Unauthorized("invalid token claims", "AUTH_INVALID_CLAIMS"))
			return
		}

		subject, _ := claims["sub"].(string)

		var roles []string
		if rolesClaim, exists := claims["roles"]; exists {
			if rolesArray, ok := rolesClaim.([]interface{}); ok {
				for _, r := range rolesArray {
					if rStr, ok := r.(string); ok {
						roles = append(roles, rStr)
					}
				}
			}
		}

		// Keycloak M2M client support (resource_access mapping using client ID / azp)
		if resourceAccess, exists := claims["resource_access"]; exists {
			if raMap, ok := resourceAccess.(map[string]interface{}); ok {
				azp, _ := claims["azp"].(string)
				if azp != "" {
					if clientData, exists := raMap[azp]; exists {
						if clientMap, ok := clientData.(map[string]interface{}); ok {
							if clientRoles, exists := clientMap["roles"]; exists {
								if rolesArray, ok := clientRoles.([]interface{}); ok {
									for _, r := range rolesArray {
										if rStr, ok := r.(string); ok {
											roles = append(roles, rStr)
										}
									}
								}
							}
						}
					}
				}
			}
		}

		// Inject security context using keys from framework/security
		ctx := context.WithValue(r.Context(), security.UserContextKey, subject)
		ctx = context.WithValue(ctx, security.RolesContextKey, roles)
		ctx = context.WithValue(ctx, security.ClaimsContextKey, claims)

		// Inject all claims as simple string keys for easy context retrieval
		for k, v := range claims {
			ctx = context.WithValue(ctx, k, v)
			// Ensure "username" and "user" are synced if "sub" is set
			if k == "sub" {
				ctx = context.WithValue(ctx, "username", v)
				ctx = context.WithValue(ctx, "user", v)
			}
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// SecurityHeadersMiddleware adds basic security headers to every response
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "0")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		w.Header().Set("X-Permitted-Cross-Domain-Policies", "none")
		// HSTS (Strict-Transport-Security)
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// isPublicPath checks if the current request path matches any of the configured public paths.
// It performs exact matching or boundary-safe prefix matching (e.g., matching subdirectories
// but not sibling directories/files sharing the same prefix).
func isPublicPath(currentPath string, publicPaths []string) bool {
	cur := currentPath
	if len(cur) > 1 && strings.HasSuffix(cur, "/") {
		cur = strings.TrimSuffix(cur, "/")
	}

	for _, path := range publicPaths {
		p := path
		if len(p) > 1 && strings.HasSuffix(p, "/") {
			p = strings.TrimSuffix(p, "/")
		}

		// Exact match or subpath match (with slash boundary)
		if cur == p || strings.HasPrefix(cur, p+"/") {
			return true
		}
	}
	return false
}

func extractToken(r *http.Request) (string, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "", fmt.Errorf("missing authorization header")
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return "", fmt.Errorf("invalid authorization format")
	}
	return parts[1], nil
}
