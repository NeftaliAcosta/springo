package web

import (
	"context"
	"crypto/subtle"
	"os"
	"crypto/rand"
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/NeftaliAcosta/springo/framework/config"
	"github.com/NeftaliAcosta/springo/framework/ioc"
	"github.com/NeftaliAcosta/springo/framework/logging"
	"github.com/NeftaliAcosta/springo/framework/scheduler"
	"github.com/NeftaliAcosta/springo/framework/version"
	"log"
	"net/http"
	"reflect"
	"runtime/pprof"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

//go:embed dashboard.html
var dashboardHTML string

// BasicAuthProperties defines basic security settings
type BasicAuthProperties struct {
	Name     string `yaml:"name"`
	Password string `yaml:"password"`
}

func (p *BasicAuthProperties) Validate() error {
	profile := strings.ToLower(strings.TrimSpace(os.Getenv("SPRINGO_PROFILES_ACTIVE")))
	if profile == "prod" || profile == "production" {
		if strings.TrimSpace(p.Password) == "" {
			return fmt.Errorf("spring.security.user.password must be explicitly set in production profile")
		}
	}
	return nil
}

// ExposureProperties controls exposed actuator endpoints
type ExposureProperties struct {
	Include string `yaml:"include"`
}

type WebProperties struct {
	Exposure ExposureProperties `yaml:"exposure"`
}

type EndpointsProperties struct {
	Web WebProperties `yaml:"web"`
}

type ManagementProperties struct {
	Endpoints EndpointsProperties `yaml:"endpoints"`
}

func init() {
	config.RegisterProperties("spring.security.user", &BasicAuthProperties{
		Name:     "admin",
		Password: "", // Empty triggers secure random generation
	})

	config.RegisterProperties("management", &ManagementProperties{
		Endpoints: EndpointsProperties{
			Web: WebProperties{
				Exposure: ExposureProperties{
					Include: "*", // Expose everything by default
				},
			},
		},
	})
}

var (
	generatedPassword     string
	generatedPasswordOnce sync.Once
	dlqRetryCallback      func(ctx context.Context, eventName string, payload string) error
	dlqRetryCallbackMu    sync.RWMutex
)

// RegisterDlqRetryCallback registers a callback to re-dispatch events, avoiding circular imports
func RegisterDlqRetryCallback(fn func(ctx context.Context, eventName string, payload string) error) {
	dlqRetryCallbackMu.Lock()
	defer dlqRetryCallbackMu.Unlock()
	dlqRetryCallback = fn
}

func getOrGenerateCredentials() (string, string) {
	props := config.Get[BasicAuthProperties]()
	if props == nil {
		props = &BasicAuthProperties{Name: "admin", Password: ""}
	}
	user := props.Name
	if user == "" {
		user = "admin"
	}
	pass := props.Password
	if pass == "" {
		generatedPasswordOnce.Do(func() {
			generatedPassword = generateRandomPassword(16)
		})
		pass = generatedPassword
	}
	return user, pass
}

func generateRandomPassword(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return string(b)
}

func isActuatorAuthenticated(r *http.Request) bool {
	expectedUser, expectedPass := getOrGenerateCredentials()
	user, pass, ok := r.BasicAuth()
	if !ok {
		return false
	}
	userMatch := subtle.ConstantTimeCompare([]byte(user), []byte(expectedUser)) == 1
	passMatch := subtle.ConstantTimeCompare([]byte(pass), []byte(expectedPass)) == 1
	return userMatch && passMatch
}

func isEndpointExposed(endpoint string) bool {
	props := config.Get[ManagementProperties]()
	include := "*"
	if props != nil && props.Endpoints.Web.Exposure.Include != "" {
		include = props.Endpoints.Web.Exposure.Include
	}
	if include == "*" {
		return true
	}
	parts := strings.Split(include, ",")
	for _, p := range parts {
		if strings.TrimSpace(p) == endpoint {
			return true
		}
	}
	return false
}

// ActuatorBasicAuthMiddleware protects sensitive actuator endpoints
func ActuatorBasicAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only intercept /actuator paths
		if !isActuatorPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		// Public health check and info routes bypass authentication
		if r.URL.Path == "/actuator/health" || r.URL.Path == "/actuator/info" {
			next.ServeHTTP(w, r)
			return
		}

		if !isActuatorAuthenticated(r) {
			w.Header().Set("WWW-Authenticate", `Basic realm="SprinGo Actuator"`)
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("401 Unauthorized\n"))
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RegisterActuatorRoutes configures management endpoints on the chi router
func RegisterActuatorRoutes(r chi.Router) {
	// Pre-generate credentials on startup to print password to console log immediately
	user, _ := getOrGenerateCredentials()
	if generatedPassword != "" {
		log.Printf("🔑 [Security] A temporary security password was generated for user '%s' (set 'spring.security.user.password' to configure)", user)
	}

	// Add Basic Auth specifically for actuator endpoints
	r.Use(ActuatorBasicAuthMiddleware)

	r.Route("/actuator", func(r chi.Router) {
		r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			info := GetHealthInfo()
			if isActuatorAuthenticated(r) {
				if info.Status == StatusDown {
					w.WriteHeader(http.StatusServiceUnavailable)
				}
				_ = json.NewEncoder(w).Encode(info)
				return
			}

			// Unauthenticated minimal health response
			if info.Status == StatusDown {
				w.WriteHeader(http.StatusServiceUnavailable)
			}
			_ = json.NewEncoder(w).Encode(map[string]HealthStatus{
				"status": info.Status,
			})
		})

		r.Get("/info", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"app": map[string]string{
					"name":        version.Name + " Application",
					"description": "Enterprise Go Framework",
				},
				"version": version.Current,
			})
		})

		r.Get("/dashboard", func(w http.ResponseWriter, r *http.Request) {
			if !isEndpointExposed("dashboard") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(dashboardHTML))
		})

		r.Get("/threaddump", func(w http.ResponseWriter, r *http.Request) {
			if !isEndpointExposed("threaddump") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_ = pprof.Lookup("goroutine").WriteTo(w, 2)
		})

		r.Get("/loggers", func(w http.ResponseWriter, r *http.Request) {
			if !isEndpointExposed("loggers") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"level": logging.GetLevel(),
			})
		})

		r.Post("/loggers", func(w http.ResponseWriter, r *http.Request) {
			if !isEndpointExposed("loggers") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			var body struct {
				ConfiguredLevel string `json:"configuredLevel"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			logging.SetLevel(body.ConfiguredLevel)
			w.WriteHeader(http.StatusOK)
		})

		r.Get("/beans", func(w http.ResponseWriter, r *http.Request) {
			if !isEndpointExposed("beans") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			beans := ioc.GetContainer().GetAllBeans()

			type beanInfo struct {
				Name  string `json:"name"`
				Type  string `json:"type"`
				Scope string `json:"scope"`
			}
			var list []beanInfo
			for name, bean := range beans {
				list = append(list, beanInfo{
					Name:  name,
					Type:  reflect.TypeOf(bean).String(),
					Scope: "singleton",
				})
			}
			_ = json.NewEncoder(w).Encode(list)
		})

		r.Get("/env", func(w http.ResponseWriter, r *http.Request) {
			if !isEndpointExposed("env") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			rawEnv := config.GetConfigProperties()
			maskedEnv := maskSensitiveData(rawEnv)
			_ = json.NewEncoder(w).Encode(maskedEnv)
		})

		// DB, ShedLock and DLQ metrics
		r.Get("/shedlock", func(w http.ResponseWriter, r *http.Request) {
			if !isEndpointExposed("shedlock") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")

			// 1. Get configured jobs from scheduler manager
			jobs := scheduler.GetSchedulerJobs()

			// 2. Query active locks from database if table exists
			db := ioc.GetContainer().GetDB()
			type shedLockRow struct {
				Name      string    `json:"name"`
				LockUntil time.Time `json:"lock_until"`
				LockedAt  time.Time `json:"locked_at"`
				LockedBy  string    `json:"locked_by"`
			}
			locksMap := make(map[string]shedLockRow)
			if db != nil && db.Migrator().HasTable("springo_shedlock") {
				var dbLocks []shedLockRow
				if err := db.Raw("SELECT name, lock_until, locked_at, locked_by FROM springo_shedlock").Scan(&dbLocks).Error; err == nil {
					for _, lock := range dbLocks {
						locksMap[lock.Name] = lock
					}
				}
			}

			// 3. Combine them
			type combinedJobInfo struct {
				Name         string    `json:"name"`
				Cron         string    `json:"cron,omitempty"`
				FixedRate    string    `json:"fixed_rate,omitempty"`
				FixedDelay   string    `json:"fixed_delay,omitempty"`
				RunOnStartup bool      `json:"run_on_startup"`
				Priority     int       `json:"priority"`
				Critical     bool      `json:"critical"`
				Enabled      bool      `json:"enabled"`
				LockEnabled  bool      `json:"lock_enabled"`
				Registered   bool      `json:"registered"`
				LockedBy     string    `json:"locked_by,omitempty"`
				LockUntil    time.Time `json:"lock_until,omitempty"`
				LastExecuted time.Time `json:"last_executed,omitempty"`
				NextExpected time.Time `json:"next_expected,omitempty"`
			}

			var result []combinedJobInfo
			for _, job := range jobs {
				lockedBy := ""
				var lockUntil time.Time
				if lock, exists := locksMap[job.Name]; exists {
					if lock.LockUntil.After(time.Now()) {
						lockedBy = lock.LockedBy
						lockUntil = lock.LockUntil
					}
				}
				result = append(result, combinedJobInfo{
					Name:         job.Name,
					Cron:         job.Cron,
					FixedRate:    job.FixedRate,
					FixedDelay:   job.FixedDelay,
					RunOnStartup: job.RunOnStartup,
					Priority:     job.Priority,
					Critical:     job.Critical,
					Enabled:      job.Enabled,
					LockEnabled:  job.LockEnabled,
					Registered:   job.Registered,
					LockedBy:     lockedBy,
					LockUntil:    lockUntil,
					LastExecuted: job.LastExecuted,
					NextExpected: job.NextExpected,
				})
			}

			_ = json.NewEncoder(w).Encode(result)
		})

		r.Post("/shedlock/trigger", func(w http.ResponseWriter, r *http.Request) {
			if !isEndpointExposed("shedlock") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			var body struct {
				JobName string `json:"jobName"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if err := scheduler.TriggerJobManually(body.JobName); err != nil {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(err.Error()))
				return
			}
			w.WriteHeader(http.StatusOK)
		})

		r.Get("/dlq", func(w http.ResponseWriter, r *http.Request) {
			if !isEndpointExposed("dlq") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			db := ioc.GetContainer().GetDB()

			type failedEventRow struct {
				ID           uint      `json:"id"`
				EventName    string    `json:"event_name"`
				Payload      string    `json:"payload"`
				ListenerName string    `json:"listener_name"`
				Error        string    `json:"error"`
				Retries      int       `json:"retries"`
				Status       string    `json:"status"`
				TraceID      string    `json:"trace_id"`
				NextRetryAt  time.Time `json:"next_retry_at"`
			}
			var list []failedEventRow
			if db != nil && db.Migrator().HasTable("springo_failed_events") {
				_ = db.Raw("SELECT id, event_name, payload, listener_name, error, retries, status, trace_id, next_retry_at FROM springo_failed_events ORDER BY id DESC").Scan(&list).Error
			}
			_ = json.NewEncoder(w).Encode(list)
		})

		r.Post("/dlq/retry", func(w http.ResponseWriter, r *http.Request) {
			if !isEndpointExposed("dlq") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			id := r.URL.Query().Get("id")
			if id == "" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			db := ioc.GetContainer().GetDB()
			if db == nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			type failedEventRow struct {
				ID        uint   `gorm:"column:id"`
				EventName string `gorm:"column:event_name"`
				Payload   string `gorm:"column:payload"`
			}
			var row failedEventRow
			if err := db.Raw("SELECT id, event_name, payload FROM springo_failed_events WHERE id = ?", id).Scan(&row).Error; err != nil || row.ID == 0 {
				w.WriteHeader(http.StatusNotFound)
				return
			}

			dlqRetryCallbackMu.RLock()
			callback := dlqRetryCallback
			dlqRetryCallbackMu.RUnlock()

			if callback == nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte("DLQ retry callback is not registered"))
				return
			}

			// Update retries count and status in DB so UI immediately reflects the retry attempt
			log.Printf("🔄 [Actuator-DLQ] Manual retry triggered for event ID %s (%s)", id, row.EventName)
			_ = db.Exec("UPDATE springo_failed_events SET retries = retries + 1, status = 'RETRYING', updated_at = ? WHERE id = ?", time.Now(), id)

			if err := callback(r.Context(), row.EventName, row.Payload); err != nil {
				log.Printf("❌ [Actuator-DLQ] Manual retry failed for event ID %s: %v", id, err)
				_ = db.Exec("UPDATE springo_failed_events SET status = 'FAILED', error = ? WHERE id = ?", err.Error(), id)
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(err.Error()))
				return
			}

			// On successful retry dispatch, remove it from DLQ
			log.Printf("✅ [Actuator-DLQ] Event ID %s successfully re-dispatched and cleared from DLQ", id)
			_ = db.Exec("DELETE FROM springo_failed_events WHERE id = ?", id)
			w.WriteHeader(http.StatusOK)
		})

		r.Post("/dlq/purge", func(w http.ResponseWriter, r *http.Request) {
			if !isEndpointExposed("dlq") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			id := r.URL.Query().Get("id")
			db := ioc.GetContainer().GetDB()
			if db == nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			if id != "" {
				_ = db.Exec("DELETE FROM springo_failed_events WHERE id = ?", id)
			} else {
				_ = db.Exec("DELETE FROM springo_failed_events")
			}
			w.WriteHeader(http.StatusOK)
		})
	})
}

func maskSensitiveData(data interface{}) interface{} {
	val := reflect.ValueOf(data)
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return nil
		}
		val = val.Elem()
	}

	switch val.Kind() {
	case reflect.Map:
		res := make(map[string]interface{})
		for _, key := range val.MapKeys() {
			kStr := fmt.Sprintf("%v", key.Interface())
			v := val.MapIndex(key).Interface()
			if isSensitiveKey(kStr) {
				res[kStr] = "******"
			} else {
				res[kStr] = maskSensitiveData(v)
			}
		}
		return res
	case reflect.Struct:
		res := make(map[string]interface{})
		t := val.Type()
		for i := 0; i < val.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" {
				continue
			}
			name := f.Tag.Get("yaml")
			if name == "" {
				name = f.Tag.Get("json")
			}
			if name == "" {
				name = strings.ToLower(f.Name)
			}
			if idx := strings.Index(name, ","); idx != -1 {
				name = name[:idx]
			}
			v := val.Field(i).Interface()
			if isSensitiveKey(name) {
				res[name] = "******"
			} else {
				res[name] = maskSensitiveData(v)
			}
		}
		return res
	case reflect.Slice:
		res := make([]interface{}, val.Len())
		for i := 0; i < val.Len(); i++ {
			res[i] = maskSensitiveData(val.Index(i).Interface())
		}
		return res
	default:
		return data
	}
}

func isSensitiveKey(key string) bool {
	k := strings.ToLower(key)
	return strings.Contains(k, "password") ||
		strings.Contains(k, "secret") ||
		strings.Contains(k, "key") ||
		strings.Contains(k, "token") ||
		strings.Contains(k, "credential") ||
		strings.Contains(k, "pwd")
}
