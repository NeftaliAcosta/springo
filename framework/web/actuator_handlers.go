package web

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"reflect"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/NeftaliAcosta/springo/framework/config"
	"github.com/NeftaliAcosta/springo/framework/ioc"
	"github.com/NeftaliAcosta/springo/framework/logging"
	"github.com/NeftaliAcosta/springo/framework/scheduler"
	"github.com/NeftaliAcosta/springo/framework/version"
	"gorm.io/gorm"
)

type shedLockRow struct {
	Name      string    `json:"name"`
	LockUntil time.Time `json:"lock_until"`
	LockedAt  time.Time `json:"locked_at"`
	LockedBy  string    `json:"locked_by"`
}

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

type dlqFailedEventRow struct {
	ID           uint      `json:"id" gorm:"column:id"`
	EventName    string    `json:"event_name" gorm:"column:event_name"`
	Payload      string    `json:"payload" gorm:"column:payload"`
	ListenerName string    `json:"listener_name" gorm:"column:listener_name"`
	Error        string    `json:"error" gorm:"column:error"`
	Retries      int       `json:"retries" gorm:"column:retries"`
	Status       string    `json:"status" gorm:"column:status"`
	TraceID      string    `json:"trace_id" gorm:"column:trace_id"`
	NextRetryAt  time.Time `json:"next_retry_at" gorm:"column:next_retry_at"`
}

type beanInfo struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Scope string `json:"scope"`
}

// HandleActuatorHealth returns health status or full health details if authenticated.
func handleActuatorHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	info := GetHealthInfo()
	if info.Status == StatusDown {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	if isActuatorAuthenticated(r) {
		_ = json.NewEncoder(w).Encode(info)
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]HealthStatus{
		"status": info.Status,
	})
}

// HandleActuatorInfo returns application metadata.
func handleActuatorInfo(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"app": map[string]string{
			"name":        version.Name + " Application",
			"description": "Enterprise Go Framework",
		},
		"version": version.Current,
	})
}

// HandleActuatorDashboard serves the embedded HTML dashboard.
func handleActuatorDashboard(w http.ResponseWriter, _ *http.Request) {
	if !isEndpointExposed("dashboard") {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(dashboardHTML))
}

// HandleActuatorDashboardCSS serves the dashboard stylesheet.
func handleActuatorDashboardCSS(w http.ResponseWriter, _ *http.Request) {
	if !isEndpointExposed("dashboard") {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(dashboardCSS))
}

// HandleActuatorDashboardJS serves the dashboard client script.
func handleActuatorDashboardJS(w http.ResponseWriter, _ *http.Request) {
	if !isEndpointExposed("dashboard") {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(dashboardJS))
}

// HandleActuatorThreadDump returns active goroutine stack traces.
func handleActuatorThreadDump(w http.ResponseWriter, _ *http.Request) {
	if !isEndpointExposed("threaddump") {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = pprof.Lookup("goroutine").WriteTo(w, 2)
}

// HandleActuatorLoggersGet returns current logging level.
func handleActuatorLoggersGet(w http.ResponseWriter, _ *http.Request) {
	if !isEndpointExposed("loggers") {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"level": logging.GetLevel(),
	})
}

// HandleActuatorLoggersPost updates logging level dynamically.
func handleActuatorLoggersPost(w http.ResponseWriter, r *http.Request) {
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
}

// HandleActuatorBeans lists all registered IoC beans.
func handleActuatorBeans(w http.ResponseWriter, _ *http.Request) {
	if !isEndpointExposed("beans") {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	beans := ioc.GetContainer().GetAllBeans()

	list := make([]beanInfo, 0, len(beans))
	for name, bean := range beans {
		list = append(list, beanInfo{
			Name:  name,
			Type:  reflect.TypeOf(bean).String(),
			Scope: "singleton",
		})
	}
	_ = json.NewEncoder(w).Encode(list)
}

// HandleActuatorEnv returns environment configuration with masked sensitive values.
func handleActuatorEnv(w http.ResponseWriter, _ *http.Request) {
	if !isEndpointExposed("env") {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	rawEnv := config.GetConfigProperties()
	maskedEnv := maskSensitiveData(rawEnv)
	_ = json.NewEncoder(w).Encode(maskedEnv)
}

func queryActiveShedLocks() map[string]shedLockRow {
	locksMap := make(map[string]shedLockRow)
	db := ioc.GetContainer().GetDB()
	if db == nil || !db.Migrator().HasTable("springo_shedlock") {
		return locksMap
	}
	var dbLocks []shedLockRow
	queryErr := db.Raw("SELECT name, lock_until, locked_at, locked_by FROM springo_shedlock").Scan(&dbLocks).Error
	if queryErr == nil {
		for _, lock := range dbLocks {
			locksMap[lock.Name] = lock
		}
	}
	return locksMap
}

func buildCombinedJobInfo(job scheduler.SchedulerJobInfo, locksMap map[string]shedLockRow) combinedJobInfo {
	lockedBy := ""
	var lockUntil time.Time
	if lock, exists := locksMap[job.Name]; exists && lock.LockUntil.After(time.Now()) {
		lockedBy = lock.LockedBy
		lockUntil = lock.LockUntil
	}
	return combinedJobInfo{
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
	}
}

// HandleActuatorShedlockGet returns configured scheduler jobs and lock status.
func handleActuatorShedlockGet(w http.ResponseWriter, _ *http.Request) {
	if !isEndpointExposed("shedlock") {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")

	jobs := scheduler.GetSchedulerJobs()
	locksMap := queryActiveShedLocks()

	result := make([]combinedJobInfo, 0, len(jobs))
	for _, job := range jobs {
		result = append(result, buildCombinedJobInfo(job, locksMap))
	}

	_ = json.NewEncoder(w).Encode(result)
}

// HandleActuatorShedlockTrigger triggers a scheduled job immediately.
func handleActuatorShedlockTrigger(w http.ResponseWriter, r *http.Request) {
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
}

// HandleActuatorDlqGet returns dead-letter queue records.
func handleActuatorDlqGet(w http.ResponseWriter, _ *http.Request) {
	if !isEndpointExposed("dlq") {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	db := ioc.GetContainer().GetDB()

	var list []dlqFailedEventRow
	if db != nil && db.Migrator().HasTable("springo_failed_events") {
		_ = db.Raw("SELECT id, event_name, payload, listener_name, error, retries, status, trace_id, " +
			"next_retry_at FROM springo_failed_events ORDER BY id DESC").Scan(&list).Error
	}
	_ = json.NewEncoder(w).Encode(list)
}

// HandleActuatorDlqRetry attempts manual retry of a dead-letter event.
func handleActuatorDlqRetry(w http.ResponseWriter, r *http.Request) {
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

	var row dlqFailedEventRow
	findErr := db.Raw("SELECT id, event_name, payload FROM springo_failed_events WHERE id = ?", id).Scan(&row).Error
	if findErr != nil || row.ID == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	executeDlqRetry(w, r, db, id, row)
}

func executeDlqRetry(w http.ResponseWriter, r *http.Request, db *gorm.DB, id string, row dlqFailedEventRow) {
	dlqRetryCallbackMu.RLock()
	callback := dlqRetryCallback
	dlqRetryCallbackMu.RUnlock()

	if callback == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("DLQ retry callback is not registered"))
		return
	}

	log.Printf("🔄 [Actuator-DLQ] Manual retry triggered for event ID %s (%s)", id, row.EventName)
	_ = db.Exec("UPDATE springo_failed_events SET retries = retries + 1, status = 'RETRYING', "+
		"updated_at = ? WHERE id = ?", time.Now(), id)

	if err := callback(r.Context(), row.EventName, row.Payload); err != nil {
		log.Printf("❌ [Actuator-DLQ] Manual retry failed for event ID %s: %v", id, err)
		_ = db.Exec("UPDATE springo_failed_events SET status = 'FAILED', error = ? WHERE id = ?", err.Error(), id)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	log.Printf("✅ [Actuator-DLQ] Event ID %s successfully re-dispatched and cleared from DLQ", id)
	_ = db.Exec("DELETE FROM springo_failed_events WHERE id = ?", id)
	w.WriteHeader(http.StatusOK)
}

// HandleActuatorDlqPurge deletes events from the DLQ.
func handleActuatorDlqPurge(w http.ResponseWriter, r *http.Request) {
	if !isEndpointExposed("dlq") {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	db := ioc.GetContainer().GetDB()
	if db == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	id := r.URL.Query().Get("id")
	if id != "" {
		_ = db.Exec("DELETE FROM springo_failed_events WHERE id = ?", id)
	} else {
		_ = db.Exec("DELETE FROM springo_failed_events")
	}
	w.WriteHeader(http.StatusOK)
}

func maskSensitiveData(data interface{}) interface{} {
	val := reflect.ValueOf(data)
	if val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return nil
		}
		val = val.Elem()
	}

	switch val.Kind() {
	case reflect.Map:
		return maskMap(val)
	case reflect.Struct:
		return maskStruct(val)
	case reflect.Slice:
		return maskSlice(val)
	default:
		return data
	}
}

func maskMap(val reflect.Value) map[string]interface{} {
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
}

func maskStruct(val reflect.Value) map[string]interface{} {
	res := make(map[string]interface{})
	t := val.Type()
	for i := 0; i < val.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" {
			continue
		}
		name := resolveFieldName(f)
		v := val.Field(i).Interface()
		if isSensitiveKey(name) {
			res[name] = "******"
		} else {
			res[name] = maskSensitiveData(v)
		}
	}
	return res
}

func resolveFieldName(f reflect.StructField) string {
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
	return name
}

func maskSlice(val reflect.Value) []interface{} {
	res := make([]interface{}, val.Len())
	for i := 0; i < val.Len(); i++ {
		res[i] = maskSensitiveData(val.Index(i).Interface())
	}
	return res
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
