package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

type userDTO struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func TestResponseCaptureMiddlewareCapturesDispatchJSON(t *testing.T) {
	router := chi.NewRouter()
	router.Use(ResponseCaptureMiddleware)

	var capturedInChain *CapturedResponse
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
			captured, ok := GetCapturedResponse(r.Context())
			if !ok || captured == nil {
				t.Fatalf("expected captured response in context")
			}
			capturedInChain = captured
		})
	})

	router.Get("/user", Dispatch(func(ctx context.Context) (any, error) {
		return userDTO{ID: 101, Name: "Neftali"}, nil
	}))

	req := httptest.NewRequest(http.MethodGet, "/user", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if capturedInChain == nil {
		t.Fatalf("expected captured response to be populated")
	}
	if capturedInChain.StatusCode != http.StatusOK {
		t.Fatalf("expected captured status 200, got %d", capturedInChain.StatusCode)
	}

	var res map[string]any
	if err := json.Unmarshal(capturedInChain.Body, &res); err != nil {
		t.Fatalf("unmarshal captured body: %v", err)
	}
	data, ok := res["data"].(map[string]any)
	if !ok || data["name"] != "Neftali" {
		t.Fatalf("unexpected captured body content: %#v", res)
	}
}

func TestResponseCaptureMiddlewareCapturesErrorHandler(t *testing.T) {
	router := chi.NewRouter()
	router.Use(ResponseCaptureMiddleware)

	var capturedInChain *CapturedResponse
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
			captured, ok := GetCapturedResponse(r.Context())
			if ok {
				capturedInChain = captured
			}
		})
	})

	router.Get("/fail", Dispatch(func(ctx context.Context) (any, error) {
		return nil, errors.New("database failure")
	}))

	req := httptest.NewRequest(http.MethodGet, "/fail", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}
	if capturedInChain == nil {
		t.Fatalf("expected captured response on error")
	}
	if capturedInChain.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected captured status 500, got %d", capturedInChain.StatusCode)
	}
	if !strings.Contains(string(capturedInChain.Body), "INTERNAL_SERVER_ERROR") {
		t.Fatalf("expected INTERNAL_SERVER_ERROR in captured body: %s", string(capturedInChain.Body))
	}
}

func TestResponseCaptureMiddlewareAllowsPostProcessingTransformations(t *testing.T) {
	router := chi.NewRouter()
	router.Use(ResponseCaptureMiddleware)

	// Middleware that modifies the captured response before flushing.
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
			captured, ok := GetCapturedResponse(r.Context())
			if !ok {
				t.Fatalf("expected captured response")
			}

			// Modify body, header, and status.
			captured.Body = []byte(`{"transformed":true}`)
			captured.Headers.Set("X-Transformed", "1")
			captured.StatusCode = http.StatusAccepted
		})
	})

	router.Get("/original", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"original":true}`)
	})

	req := httptest.NewRequest(http.MethodGet, "/original", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", rec.Code)
	}
	if rec.Header().Get("X-Transformed") != "1" {
		t.Fatalf("expected X-Transformed header to be set on client response")
	}
	if rec.Body.String() != `{"transformed":true}` {
		t.Fatalf("expected transformed body, got %q", rec.Body.String())
	}
}

func TestGetCapturedResponseNilContext(t *testing.T) {
	var nilCtx context.Context
	captured, ok := GetCapturedResponse(nilCtx)
	if ok || captured != nil {
		t.Fatalf("expected false and nil for nil context")
	}

	captured, ok = GetCapturedResponse(context.Background())
	if ok || captured != nil {
		t.Fatalf("expected false and nil for empty context")
	}
}

type flusherRecorder struct {
	*httptest.ResponseRecorder
	flushed bool
}

func (f *flusherRecorder) Flush() {
	f.flushed = true
}

func TestResponseCaptureWriterFlusherAndUnwrap(t *testing.T) {
	rec := &flusherRecorder{ResponseRecorder: httptest.NewRecorder()}
	captured := &CapturedResponse{}
	cw := newResponseCaptureWriter(rec, captured)

	cw.Flush()
	if !rec.flushed {
		t.Fatalf("expected underlying flusher to be invoked")
	}

	if cw.Unwrap() != rec {
		t.Fatalf("expected Unwrap to return underlying recorder")
	}
}
