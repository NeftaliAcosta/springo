package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/NeftaliAcosta/springo/framework/config"

	"github.com/felixge/httpsnoop"
)



const (
	TraceIDKey  string = "springo_trace_id"
	SpanIDKey   string = "springo_span_id"
	TraceHeader string = "X-Trace-ID"
)

// TelemetryProperties defines configurations for OpenTelemetry/Zipkin integration
type TelemetryProperties struct {
	Enabled     bool   `yaml:"enabled"`
	ServiceName string `yaml:"service-name"`
	Zipkin      struct {
		Enabled       bool   `yaml:"enabled"`
		Endpoint      string `yaml:"endpoint"`
		BatchSize     int    `yaml:"batch-size"`
		FlushInterval string `yaml:"flush-interval"`
	} `yaml:"zipkin"`
}

func init() {
	config.RegisterProperties("spring.telemetry", &TelemetryProperties{
		Enabled:     false,
		ServiceName: "springo-app",
		Zipkin: struct {
			Enabled       bool   `yaml:"enabled"`
			Endpoint      string `yaml:"endpoint"`
			BatchSize     int    `yaml:"batch-size"`
			FlushInterval string `yaml:"flush-interval"`
		}{
			Enabled:       false,
			Endpoint:      "http://localhost:9411/api/v2/spans",
			BatchSize:     100,
			FlushInterval: "5s",
		},
	})
}

// w3cTraceParent parses traceparent header: version-trace_id-parent_id-trace_flags
type w3cTraceParent struct {
	Version  string
	TraceID  string
	ParentID string
	Flags    string
}

func parseTraceParent(header string) (*w3cTraceParent, bool) {
	if header == "" {
		return nil, false
	}
	parts := strings.Split(header, "-")
	if len(parts) != 4 {
		return nil, false
	}
	// Validate length of version, trace_id, parent_id, flags
	if len(parts[0]) != 2 || len(parts[1]) != 32 || len(parts[2]) != 16 || len(parts[3]) != 2 {
		return nil, false
	}
	// Validate they are hex
	for _, p := range parts {
		for _, r := range p {
			if !unicode.Is(unicode.Hex_Digit, r) {
				return nil, false
			}
		}
	}
	return &w3cTraceParent{
		Version:  parts[0],
		TraceID:  parts[1],
		ParentID: parts[2],
		Flags:    parts[3],
	}, true
}

func generateTraceID() string {
	bytes := make([]byte, 16)
	_, _ = rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func generateSpanID() string {
	bytes := make([]byte, 8)
	_, _ = rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// TracingMiddleware ensures every request has a unique Trace ID for W3C compliance & observability
func TracingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := ""
		parentID := ""

		// 1. Try parsing W3C traceparent
		tpHeader := r.Header.Get("traceparent")
		if tp, ok := parseTraceParent(tpHeader); ok {
			traceID = tp.TraceID
			parentID = tp.ParentID
		}

		// 2. Try legacy X-Trace-ID for backward compatibility
		if traceID == "" {
			traceID = r.Header.Get(TraceHeader)
		}

		// 3. Fallback: generate new trace ID
		if traceID == "" {
			traceID = generateTraceID()
		}
		spanID := generateSpanID()

		// Set W3C traceparent and legacy X-Trace-ID on response
		w.Header().Set("traceparent", fmt.Sprintf("00-%s-%s-01", traceID, spanID))
		w.Header().Set(TraceHeader, traceID)

		ctx := r.Context()
		ctx = context.WithValue(ctx, TraceIDKey, traceID)
		ctx = context.WithValue(ctx, SpanIDKey, spanID)

		startTime := time.Now()

		var status = http.StatusOK
		var statusCaptured bool
		var statusMu sync.Mutex

		wrapped := httpsnoop.Wrap(w, httpsnoop.Hooks{
			WriteHeader: func(next httpsnoop.WriteHeaderFunc) httpsnoop.WriteHeaderFunc {
				return func(code int) {
					statusMu.Lock()
					if !statusCaptured {
						status = code
						statusCaptured = true
					}
					statusMu.Unlock()
					next(code)
				}
			},
			Write: func(next httpsnoop.WriteFunc) httpsnoop.WriteFunc {
				return func(b []byte) (int, error) {
					statusMu.Lock()
					if !statusCaptured {
						status = http.StatusOK
						statusCaptured = true
					}
					statusMu.Unlock()
					return next(b)
				}
			},
		})

		next.ServeHTTP(wrapped, r.WithContext(ctx))

		// Export span if telemetry is active
		duration := time.Since(startTime)
		captureHTTPSpan(ctx, r.Method, r.URL.Path, status, duration, startTime, parentID)
	})
}

// GetTraceID extracts the Trace ID from the context
func GetTraceID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if traceID, ok := ctx.Value(TraceIDKey).(string); ok {
		return traceID
	}
	return ""
}

// WithTraceID injects a Trace ID into the context
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, TraceIDKey, traceID)
}

// ─── Zipkin Exporter & Spans ───────────────────────────────────────────────

type ZipkinEndpoint struct {
	ServiceName string `json:"serviceName"`
}

type ZipkinSpan struct {
	TraceID       string            `json:"traceId"`
	ID            string            `json:"id"`
	ParentID      string            `json:"parentId,omitempty"`
	Name          string            `json:"name"`
	Timestamp     int64             `json:"timestamp"` // micro seconds
	Duration      int64             `json:"duration"`  // micro seconds
	LocalEndpoint ZipkinEndpoint    `json:"localEndpoint"`
	Tags          map[string]string `json:"tags,omitempty"`
}

type spanBatchExporter struct {
	queue         chan ZipkinSpan
	client        *http.Client
	endpoint      string
	batchSize     int
	flushInterval time.Duration
	stopChan      chan struct{}
	wg            sync.WaitGroup
	enabled       bool
}

var (
	globalExporter *spanBatchExporter
	exporterMutex  sync.RWMutex
)

// InitializeTelemetry starts the background telemetry exporter routine
func InitializeTelemetry(props *TelemetryProperties) {
	exporterMutex.Lock()
	defer exporterMutex.Unlock()

	if globalExporter != nil {
		close(globalExporter.stopChan)
		globalExporter.wg.Wait()
	}

	flushDur, err := time.ParseDuration(props.Zipkin.FlushInterval)
	if err != nil {
		flushDur = 5 * time.Second
	}
	batchSize := props.Zipkin.BatchSize
	if batchSize <= 0 {
		batchSize = 100
	}

	globalExporter = &spanBatchExporter{
		queue:         make(chan ZipkinSpan, 10000),
		client:        &http.Client{Timeout: 5 * time.Second},
		endpoint:      props.Zipkin.Endpoint,
		batchSize:     batchSize,
		flushInterval: flushDur,
		stopChan:      make(chan struct{}),
		enabled:       props.Enabled && props.Zipkin.Enabled,
	}

	if globalExporter.enabled {
		globalExporter.wg.Add(1)
		go globalExporter.runWorker(props.ServiceName)
	}
}

// CloseTelemetry shuts down the telemetry exporter cleanly
func CloseTelemetry() {
	exporterMutex.Lock()
	defer exporterMutex.Unlock()

	if globalExporter != nil {
		close(globalExporter.stopChan)
		globalExporter.wg.Wait()
		globalExporter = nil
	}
}

func (e *spanBatchExporter) runWorker(serviceName string) {
	defer e.wg.Done()

	ticker := time.NewTicker(e.flushInterval)
	defer ticker.Stop()

	var batch []ZipkinSpan

	flush := func() {
		if len(batch) == 0 {
			return
		}

		payload, err := json.Marshal(batch)
		if err != nil {
			batch = nil
			return
		}

		req, err := http.NewRequest(http.MethodPost, e.endpoint, strings.NewReader(string(payload)))
		if err != nil {
			batch = nil
			return
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := e.client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
		}

		batch = nil
	}

	for {
		select {
		case span := <-e.queue:
			span.LocalEndpoint.ServiceName = serviceName
			batch = append(batch, span)
			if len(batch) >= e.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-e.stopChan:
			// Flush remaining spans
			flushLimit := len(e.queue)
		flushLoop:
			for i := 0; i < flushLimit; i++ {
				select {
				case span := <-e.queue:
					span.LocalEndpoint.ServiceName = serviceName
					batch = append(batch, span)
				default:
					break flushLoop
				}
			}
			flush()
			return
		}
	}
}

func captureHTTPSpan(ctx context.Context, method, path string, status int, duration time.Duration, startTime time.Time, parentID string) {
	exporterMutex.RLock()
	defer exporterMutex.RUnlock()

	if globalExporter == nil || !globalExporter.enabled {
		return
	}

	traceID := GetTraceID(ctx)
	spanID, _ := ctx.Value(SpanIDKey).(string)
	if traceID == "" || spanID == "" {
		return
	}

	span := ZipkinSpan{
		TraceID:   traceID,
		ID:        spanID,
		ParentID:  parentID,
		Name:      fmt.Sprintf("HTTP %s %s", method, path),
		Timestamp: startTime.UnixNano() / 1000,
		Duration:  duration.Nanoseconds() / 1000,
		Tags: map[string]string{
			"http.method":      method,
			"http.path":        path,
			"http.status_code": strconv.Itoa(status),
		},
	}

	select {
	case globalExporter.queue <- span:
	default:
		// Queue full, drop to prevent blocking
	}
}
