package security

import (
	"github.com/NeftaliAcosta/springo/framework/web"
	"net/http"
)

// JwtAuthMiddleware is now a bridge to the framework's core security middleware.
// This reduces cognitive complexity and delegates responsibility to the Kernel.
func JwtAuthMiddleware(next http.Handler) http.Handler {
	return web.AuthMiddleware(next)
}
