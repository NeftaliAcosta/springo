package web

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/NeftaliAcosta/springo/framework/logging"
	"github.com/go-chi/chi/v5/middleware"
)

// StructuredLoggerMiddleware logs HTTP requests using structured slog with subsystem tag and query redaction.
func StructuredLoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()

		defer func() {
			duration := time.Since(start)
			sanitizedURI := sanitizeRequestURI(r.URL)

			slog.Info(fmt.Sprintf("%s %s %d", r.Method, sanitizedURI, ww.Status()),
				slog.String(logging.SubsystemKey, logging.FrameworkSubsystem),
				slog.String("method", r.Method),
				slog.String("path", sanitizedURI),
				slog.Int("status", ww.Status()),
				slog.Duration("duration", duration),
				slog.Int("bytes", ww.BytesWritten()),
				slog.String("remote_addr", r.RemoteAddr),
				slog.String("trace_id", GetTraceID(r.Context())),
			)
		}()

		next.ServeHTTP(ww, r)
	})
}

func sanitizeRequestURI(u *url.URL) string {
	if u == nil {
		return ""
	}
	if u.RawQuery == "" {
		return u.Path
	}

	query := u.Query()
	for param := range query {
		if isSensitiveParam(param) {
			query.Set(param, "******")
		}
	}
	return u.Path + "?" + query.Encode()
}

func isSensitiveParam(param string) bool {
	p := strings.ToLower(param)
	return strings.Contains(p, "token") ||
		strings.Contains(p, "secret") ||
		strings.Contains(p, "password") ||
		strings.Contains(p, "key") ||
		strings.Contains(p, "auth") ||
		strings.Contains(p, "code")
}
