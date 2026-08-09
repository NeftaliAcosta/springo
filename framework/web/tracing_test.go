package web

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NeftaliAcosta/springo/framework/logging"

	"github.com/stretchr/testify/assert"
)

func TestParseTraceParent(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		expected *w3cTraceParent
		ok       bool
	}{
		{
			name:   "Valid W3C traceparent header",
			header: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
			expected: &w3cTraceParent{
				Version:  "00",
				TraceID:  "4bf92f3577b34da6a3ce929d0e0e4736",
				ParentID: "00f067aa0ba902b7",
				Flags:    "01",
			},
			ok: true,
		},
		{
			name:   "Invalid traceparent parts count",
			header: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7",
			ok:     false,
		},
		{
			name:   "Invalid trace ID length",
			header: "00-4bf92f3577b34da6a3ce9-00f067aa0ba902b7-01",
			ok:     false,
		},
		{
			name:   "Invalid span ID length",
			header: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba9-01",
			ok:     false,
		},
		{
			name:   "Non-hex characters in trace ID",
			header: "00-4bf92f3577b34da6a3ce929d0e0e473z-00f067aa0ba902b7-01",
			ok:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, ok := parseTraceParent(tt.header)
			assert.Equal(t, tt.ok, ok)
			if tt.ok {
				assert.Equal(t, tt.expected, res)
			}
		})
	}
}
func TestTracingMiddleware_GenerateNew(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test-path", nil)
	w := httptest.NewRecorder()

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := GetTraceID(r.Context())
		spanID, _ := r.Context().Value(SpanIDKey).(string)

		assert.NotEmpty(t, traceID)
		assert.Len(t, traceID, 32)
		assert.NotEmpty(t, spanID)
		assert.Len(t, spanID, 16)

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	TracingMiddleware(nextHandler).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// Validate headers in response
	tpHeader := w.Header().Get("traceparent")
	assert.NotEmpty(t, tpHeader)
	assert.True(t, strings.HasPrefix(tpHeader, "00-"))

	legacyHeader := w.Header().Get(TraceHeader)
	assert.NotEmpty(t, legacyHeader)
	assert.Len(t, legacyHeader, 32)
	assert.Contains(t, tpHeader, legacyHeader)
}

func TestTracingMiddleware_PropagateW3C(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test-path", nil)
	// Inject traceparent from upstream
	upstreamTrace := "4bf92f3577b34da6a3ce929d0e0e4736"
	upstreamSpan := "00f067aa0ba902b7"
	req.Header.Set("traceparent", "00-"+upstreamTrace+"-"+upstreamSpan+"-01")

	w := httptest.NewRecorder()

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := GetTraceID(r.Context())
		spanID, _ := r.Context().Value(SpanIDKey).(string)

		// Trace ID must remain unchanged (propagated)
		assert.Equal(t, upstreamTrace, traceID)
		// Span ID must be a newly generated span ID for this request boundary
		assert.NotEmpty(t, spanID)
		assert.NotEqual(t, upstreamSpan, spanID)

		w.WriteHeader(http.StatusOK)
	})

	TracingMiddleware(nextHandler).ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, upstreamTrace, w.Header().Get(TraceHeader))
}

func TestTracingMiddleware_PropagateLegacy(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test-path", nil)
	upstreamTrace := "12345678901234567890123456789012"
	req.Header.Set(TraceHeader, upstreamTrace)

	w := httptest.NewRecorder()

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := GetTraceID(r.Context())
		assert.Equal(t, upstreamTrace, traceID)
		w.WriteHeader(http.StatusOK)
	})

	TracingMiddleware(nextHandler).ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestContextLogger_TraceAndSpan(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(&logging.ContextHandler{
		Handler: slog.NewTextHandler(&buf, nil),
	})

	ctx := context.Background()
	ctx = context.WithValue(ctx, TraceIDKey, "my-trace-id-123")
	ctx = context.WithValue(ctx, SpanIDKey, "my-span-id-456")

	logger.InfoContext(ctx, "hello telemetry")

	logLine := buf.String()
	assert.Contains(t, logLine, "trace_id=my-trace-id-123")
	assert.Contains(t, logLine, "span_id=my-span-id-456")
}

func TestTelemetry_ZipkinExportIntegration(t *testing.T) {
	var receivedMutex sync.Mutex
	var receivedSpans []ZipkinSpan

	// Start a mock Zipkin server endpoint
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)

		var spans []ZipkinSpan
		err = json.Unmarshal(body, &spans)
		assert.NoError(t, err)

		receivedMutex.Lock()
		receivedSpans = append(receivedSpans, spans...)
		receivedMutex.Unlock()

		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	// Configure telemetry properties
	props := &TelemetryProperties{
		Enabled:     true,
		ServiceName: "test-service",
	}
	props.Zipkin.Enabled = true
	props.Zipkin.Endpoint = server.URL
	props.Zipkin.BatchSize = 1
	props.Zipkin.FlushInterval = "10ms"

	InitializeTelemetry(props)
	defer CloseTelemetry()

	// Trigger a request using TracingMiddleware
	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	w := httptest.NewRecorder()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	TracingMiddleware(next).ServeHTTP(w, req)

	// Wait for queue flush to background goroutine
	time.Sleep(50 * time.Millisecond)

	receivedMutex.Lock()
	defer receivedMutex.Unlock()

	assert.NotEmpty(t, receivedSpans)
	assert.Equal(t, 1, len(receivedSpans))
	span := receivedSpans[0]
	assert.Equal(t, "HTTP GET /api/users", span.Name)
	assert.Equal(t, "test-service", span.LocalEndpoint.ServiceName)
	assert.NotEmpty(t, span.TraceID)
	assert.NotEmpty(t, span.ID)
	assert.Equal(t, "200", span.Tags["http.status_code"])
	assert.Equal(t, "GET", span.Tags["http.method"])
	assert.Equal(t, "/api/users", span.Tags["http.path"])
}

type mockFlusherHijackerWriter struct {
	httptest.ResponseRecorder
	flushed  bool
	hijacked bool
}

func (m *mockFlusherHijackerWriter) Flush() {
	m.flushed = true
}

func (m *mockFlusherHijackerWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	m.hijacked = true
	return nil, nil, nil
}

func TestTracingMiddleware_PreservesResponseWriterInterfaces(t *testing.T) {
	mw := &mockFlusherHijackerWriter{
		ResponseRecorder: *httptest.NewRecorder(),
	}

	req := httptest.NewRequest(http.MethodGet, "/test-path", nil)

	var nextCalled bool
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true

		flusher, isFlusher := w.(http.Flusher)
		assert.True(t, isFlusher)
		if isFlusher {
			flusher.Flush()
			assert.True(t, mw.flushed)
		}

		hijacker, isHijacker := w.(http.Hijacker)
		assert.True(t, isHijacker)
		if isHijacker {
			_, _, err := hijacker.Hijack()
			assert.NoError(t, err)
			assert.True(t, mw.hijacked)
		}

		w.WriteHeader(http.StatusOK)
	})

	TracingMiddleware(nextHandler).ServeHTTP(mw, req)
	assert.True(t, nextCalled)
}

type bareResponseWriter struct {
	header http.Header
}

func (b *bareResponseWriter) Header() http.Header {
	if b.header == nil {
		b.header = make(http.Header)
	}
	return b.header
}
func (b *bareResponseWriter) Write(buf []byte) (int, error) {
	return len(buf), nil
}
func (b *bareResponseWriter) WriteHeader(statusCode int) {}

type flusherOnlyWriter struct {
	bareResponseWriter
	flushed bool
}

func (f *flusherOnlyWriter) Flush() {
	f.flushed = true
}

type pusherOnlyWriter struct {
	bareResponseWriter
	pushed bool
}

func (p *pusherOnlyWriter) Push(target string, opts *http.PushOptions) error {
	p.pushed = true
	return nil
}

type readerFromOnlyWriter struct {
	bareResponseWriter
	readFrom bool
}

func (r *readerFromOnlyWriter) ReadFrom(src io.Reader) (int64, error) {
	r.readFrom = true
	return 0, nil
}

func TestTracingMiddleware_CapabilityInterfaceMatching(t *testing.T) {
	// Case 1: original ResponseWriter implements NONE of the optional interfaces
	{
		req := httptest.NewRequest(http.MethodGet, "/test-path", nil)
		bw := &bareResponseWriter{}
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, isFlusher := w.(http.Flusher)
			_, isHijacker := w.(http.Hijacker)
			_, isPusher := w.(http.Pusher)
			_, isReaderFrom := w.(io.ReaderFrom)

			assert.False(t, isFlusher, "wrapper should not implement http.Flusher")
			assert.False(t, isHijacker, "wrapper should not implement http.Hijacker")
			assert.False(t, isPusher, "wrapper should not implement http.Pusher")
			assert.False(t, isReaderFrom, "wrapper should not implement io.ReaderFrom")
		})
		TracingMiddleware(nextHandler).ServeHTTP(bw, req)
	}

	// Case 2: original ResponseWriter implements ONLY Flusher
	{
		req := httptest.NewRequest(http.MethodGet, "/test-path", nil)
		f := &flusherOnlyWriter{}
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			flusher, isFlusher := w.(http.Flusher)
			_, isHijacker := w.(http.Hijacker)
			_, isPusher := w.(http.Pusher)
			_, isReaderFrom := w.(io.ReaderFrom)

			assert.True(t, isFlusher)
			assert.False(t, isHijacker)
			assert.False(t, isPusher)
			assert.False(t, isReaderFrom)

			if isFlusher {
				flusher.Flush()
				assert.True(t, f.flushed)
			}
		})
		TracingMiddleware(nextHandler).ServeHTTP(f, req)
	}

	// Case 3: original ResponseWriter implements ONLY Pusher
	{
		req := httptest.NewRequest(http.MethodGet, "/test-path", nil)
		p := &pusherOnlyWriter{}
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, isFlusher := w.(http.Flusher)
			_, isHijacker := w.(http.Hijacker)
			pusher, isPusher := w.(http.Pusher)
			_, isReaderFrom := w.(io.ReaderFrom)

			assert.False(t, isFlusher)
			assert.False(t, isHijacker)
			assert.True(t, isPusher)
			assert.False(t, isReaderFrom)

			if isPusher {
				_ = pusher.Push("/static", nil)
				assert.True(t, p.pushed)
			}
		})
		TracingMiddleware(nextHandler).ServeHTTP(p, req)
	}

	// Case 4: original ResponseWriter implements ONLY ReaderFrom
	{
		req := httptest.NewRequest(http.MethodGet, "/test-path", nil)
		rf := &readerFromOnlyWriter{}
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, isFlusher := w.(http.Flusher)
			_, isHijacker := w.(http.Hijacker)
			_, isPusher := w.(http.Pusher)
			readerFrom, isReaderFrom := w.(io.ReaderFrom)

			assert.False(t, isFlusher)
			assert.False(t, isHijacker)
			assert.False(t, isPusher)
			assert.True(t, isReaderFrom)

			if isReaderFrom {
				_, _ = readerFrom.ReadFrom(strings.NewReader("data"))
				assert.True(t, rf.readFrom)
			}
		})
		TracingMiddleware(nextHandler).ServeHTTP(rf, req)
	}
}

func TestTracingMiddleware_SpanStatusValidation(t *testing.T) {
	// Start Zipkin exporter mock to inspect captured status
	var receivedMutex sync.Mutex
	var lastStatus string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		var spans []ZipkinSpan
		err = json.Unmarshal(body, &spans)
		assert.NoError(t, err)

		receivedMutex.Lock()
		if len(spans) > 0 {
			lastStatus = spans[0].Tags["http.status_code"]
		}
		receivedMutex.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	props := &TelemetryProperties{
		Enabled:     true,
		ServiceName: "test-service",
	}
	props.Zipkin.Enabled = true
	props.Zipkin.Endpoint = server.URL
	props.Zipkin.BatchSize = 1
	props.Zipkin.FlushInterval = "10ms"

	InitializeTelemetry(props)
	defer CloseTelemetry()

	// Scenario A: WriteHeader is called multiple times. First one (e.g. 400 Bad Request) must be recorded.
	{
		req := httptest.NewRequest(http.MethodGet, "/multiple-header", nil)
		w := httptest.NewRecorder()
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.WriteHeader(http.StatusInternalServerError)
		})
		TracingMiddleware(next).ServeHTTP(w, req)

		time.Sleep(30 * time.Millisecond) // wait for queue flush
		receivedMutex.Lock()
		assert.Equal(t, "400", lastStatus, "Should capture first status code of multiple WriteHeader calls")
		receivedMutex.Unlock()
	}

	// Scenario B: Write is called without calling WriteHeader. Default 200 OK must be recorded.
	{
		req := httptest.NewRequest(http.MethodGet, "/write-only", nil)
		w := httptest.NewRecorder()
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello world"))
		})
		TracingMiddleware(next).ServeHTTP(w, req)

		time.Sleep(30 * time.Millisecond) // wait for queue flush
		receivedMutex.Lock()
		assert.Equal(t, "200", lastStatus, "Should capture status 200 when Write is called without WriteHeader")
		receivedMutex.Unlock()
	}
}
