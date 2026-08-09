package security

import (
	"github.com/NeftaliAcosta/springo/framework/web"
	"net/http"
)

// SecurityHeaders is now a bridge to the framework's core security headers middleware.
func SecurityHeaders(next http.Handler) http.Handler {
	return web.SecurityHeadersMiddleware(next)
}
