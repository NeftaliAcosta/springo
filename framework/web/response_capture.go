package web

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
)

type capturedResponseKeyType struct{}

var capturedResponseKey = capturedResponseKeyType{}
var responseWriterPool = sync.Pool{New: func() any { return &responseCaptureWriter{} }}

// CapturedResponse holds buffered response data accessible in the middleware chain.
type CapturedResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

// GetCapturedResponse retrieves the active CapturedResponse from the context if present.
func GetCapturedResponse(ctx context.Context) (*CapturedResponse, bool) {
	if ctx == nil {
		return nil, false
	}
	captured, ok := ctx.Value(capturedResponseKey).(*CapturedResponse)
	return captured, ok && captured != nil
}

// ResponseCaptureMiddleware wraps ResponseWriter to buffer status, headers, and body for post-processing.
func ResponseCaptureMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured := &CapturedResponse{StatusCode: http.StatusOK, Headers: make(http.Header)}
		pooled := requestOptimizationEnabled()
		ctx := context.WithValue(r.Context(), capturedResponseKey, captured)
		cw := newResponseCaptureWriter(w, captured)
		if pooled {
			cw = responseWriterPool.Get().(*responseCaptureWriter)
			cw.underlying = w
			cw.captured = captured
		}

		next.ServeHTTP(cw, r.WithContext(ctx))

		cw.flushToOriginal()
		if pooled {
			cw.reset()
			responseWriterPool.Put(cw)
		}
	})
}

// ResponseCaptureWriter buffers the response output and header mutations.
type responseCaptureWriter struct {
	underlying  http.ResponseWriter
	captured    *CapturedResponse
	wroteHeader bool
	hijacked    bool
	mu          sync.Mutex
}

func (w *responseCaptureWriter) reset() {
	w.underlying = nil
	w.captured = nil
	w.wroteHeader = false
	w.hijacked = false
}

func newResponseCaptureWriter(w http.ResponseWriter, captured *CapturedResponse) *responseCaptureWriter {
	if captured.Headers == nil {
		captured.Headers = make(http.Header)
	}
	return &responseCaptureWriter{
		underlying: w,
		captured:   captured,
	}
}

// Header returns the captured response header map.
func (w *responseCaptureWriter) Header() http.Header {
	return w.captured.Headers
}

// WriteHeader buffers the HTTP status code.
func (w *responseCaptureWriter) WriteHeader(statusCode int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.captured.StatusCode = statusCode
}

// MaxResponseCaptureBytes defines the upper limit for buffering response bodies in memory (10MB).
const MaxResponseCaptureBytes = 10 * 1024 * 1024

// Write buffers payload bytes in memory up to MaxResponseCaptureBytes.
func (w *responseCaptureWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.wroteHeader {
		w.wroteHeader = true
		w.captured.StatusCode = http.StatusOK
	}

	remaining := MaxResponseCaptureBytes - len(w.captured.Body)
	if remaining > 0 {
		toAppend := b
		if len(toAppend) > remaining {
			toAppend = toAppend[:remaining]
		}
		w.captured.Body = append(w.captured.Body, toAppend...)
	}

	return len(b), nil
}

// Flush forwards flush events to the underlying ResponseWriter after synchronizing headers and body.
func (w *responseCaptureWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.hijacked {
		return
	}
	w.syncHeaders()
	w.syncStatusCode()
	w.syncBody()
	if flusher, ok := w.underlying.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Hijack delegates connection hijacking to the underlying ResponseWriter if supported.
func (w *responseCaptureWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if hijacker, ok := w.underlying.(http.Hijacker); ok {
		w.hijacked = true
		return hijacker.Hijack()
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not implement http.Hijacker")
}

// Unwrap returns the underlying ResponseWriter.
func (w *responseCaptureWriter) Unwrap() http.ResponseWriter {
	return w.underlying
}

// FlushToOriginal synchronizes buffered headers, status code, and payload to the underlying writer.
func (w *responseCaptureWriter) flushToOriginal() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.hijacked {
		return
	}

	w.syncHeaders()
	w.syncStatusCode()
	w.syncBody()
}

func (w *responseCaptureWriter) syncHeaders() {
	if w.captured.Headers.Get("Content-Length") != "" {
		w.captured.Headers.Set("Content-Length", strconv.Itoa(len(w.captured.Body)))
	}

	targetHeader := w.underlying.Header()
	for key, values := range w.captured.Headers {
		targetHeader[key] = values
	}
}

func (w *responseCaptureWriter) syncStatusCode() {
	statusCode := w.captured.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	w.underlying.WriteHeader(statusCode)
}

func (w *responseCaptureWriter) syncBody() {
	if len(w.captured.Body) > 0 {
		_, _ = w.underlying.Write(w.captured.Body)
	}
}
