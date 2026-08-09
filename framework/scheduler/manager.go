package scheduler

import (
	"context"
	"fmt"
	"github.com/NeftaliAcosta/springo/framework/config"
	"github.com/NeftaliAcosta/springo/framework/ioc"
	"log"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

// JobFunc is the signature for any scheduled task
type JobFunc func(ctx context.Context) error

type taskDefinition struct {
	name string
	fn   JobFunc
}

var (
	registeredTasks = make(map[string]taskDefinition)
	mu              sync.RWMutex
	cronManager     *cron.Cron
	instanceID      string

	// Lifecycle control
	schedulerStopChan chan struct{}
	schedulerWg       sync.WaitGroup
	schedulerRunning  bool
	schedulerMu       sync.Mutex
)

func init() {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		instanceID = "unknown-instance-" + uuid.New().String()
	} else {
		instanceID = hostname
	}
}

// Register adds a new job to the manager.
func Register(name string, fn JobFunc) {
	mu.Lock()
	defer mu.Unlock()
	registeredTasks[name] = taskDefinition{
		name: name,
		fn:   fn,
	}
}

// BackupRegistrations returns a function that restores the registered tasks to their current state.
func BackupRegistrations() func() {
	mu.Lock()
	defer mu.Unlock()

	backup := make(map[string]taskDefinition)
	for k, v := range registeredTasks {
		backup[k] = v
	}

	return func() {
		mu.Lock()
		defer mu.Unlock()
		registeredTasks = make(map[string]taskDefinition)
		for k, v := range backup {
			registeredTasks[k] = v
		}
	}
}

type startupJob struct {
	name     string
	conf     JobConf
	taskFunc JobFunc
}

// RunStartupTasks executes jobs marked as run-on-startup sequentially by priority.
// RunStartupTasksE executes jobs marked as run-on-startup sequentially by priority.
// Returns an error if any critical startup job fails.
func RunStartupTasksE() error {
	props := config.Get[SchedulerProperties]()
	if props == nil || !props.Enabled {
		return nil
	}

	startupJobs := collectStartupJobs(props)
	if len(startupJobs) == 0 {
		return nil
	}

	sortStartupJobs(startupJobs)

	log.Println("[SprinGo Scheduler] 🚀 Executing critical startup sequence...")
	for _, job := range startupJobs {
		log.Printf("[SprinGo Scheduler] -> Running startup task: %s (Priority: %d)", job.name, job.conf.Priority)
		err := executeJobByConfig(job)
		if err != nil {
			if job.conf.Critical {
				return fmt.Errorf("critical startup task failed: %s - %w", job.name, err)
			}
			log.Printf("[SprinGo Scheduler] ❌ Non-critical startup task failed: %s - %v", job.name, err)
		}
	}
	return nil
}

// RunStartupTasks executes jobs marked as run-on-startup sequentially by priority.
func RunStartupTasks() {
	if err := RunStartupTasksE(); err != nil {
		log.Fatal(err)
	}
}

func sortStartupJobs(jobs []startupJob) {
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].conf.Priority < jobs[j].conf.Priority
	})
}

func collectStartupJobs(props *SchedulerProperties) []startupJob {
	var startupJobs []startupJob
	mu.RLock()
	defer mu.RUnlock()

	for name, conf := range props.Jobs {
		if job, exists := getStartupJob(name, conf); exists {
			startupJobs = append(startupJobs, job)
		}
	}
	return startupJobs
}

func getStartupJob(name string, conf JobConf) (startupJob, bool) {
	if !conf.Enabled || !conf.RunOnStartup {
		return startupJob{}, false
	}
	if task, exists := registeredTasks[name]; exists {
		return startupJob{
			name:     name,
			conf:     conf,
			taskFunc: task.fn,
		}, true
	}
	return startupJob{}, false
}

func executeJobByConfig(job startupJob) error {
	if job.conf.Lock.Enabled {
		return executeWithLock(job.name, job.conf, job.taskFunc)
	}
	return executeWithRecover(job.name, job.taskFunc)
}

// StartBackgroundJobs launches all enabled jobs in background according to their schedule.
func StartBackgroundJobs() {
	props := config.Get[SchedulerProperties]()
	if props == nil || !props.Enabled {
		return
	}

	schedulerMu.Lock()
	if schedulerRunning {
		schedulerMu.Unlock()
		return // Already running
	}
	schedulerStopChan = make(chan struct{})
	schedulerRunning = true
	schedulerMu.Unlock()

	// We use Seconds field support for higher precision
	cronManager = cron.New(cron.WithSeconds())

	mu.RLock()
	defer mu.RUnlock()

	for name, conf := range props.Jobs {
		startSingleBackgroundJob(name, conf)
	}

	cronManager.Start()
	log.Println("[SprinGo Scheduler] ✅ Background jobs engine active")
}

func startSingleBackgroundJob(name string, conf JobConf) {
	if !conf.Enabled {
		return
	}

	task, exists := registeredTasks[name]
	if !exists {
		log.Printf("[SprinGo Scheduler] ⚠️  Warning: Job '%s' configured in YAML but not registered in code", name)
		return
	}

	runner := wrapJobRunner(name, conf, task.fn)
	scheduleJobByConfig(name, conf, runner)
}

func wrapJobRunner(name string, conf JobConf, fn JobFunc) JobFunc {
	if !conf.Lock.Enabled {
		return fn
	}
	return func(ctx context.Context) error {
		return executeWithLock(name, conf, fn)
	}
}

func scheduleJobByConfig(name string, conf JobConf, runner JobFunc) {
	if conf.Cron != "" {
		scheduleCron(name, conf.Cron, runner)
	} else if conf.FixedRate != "" {
		scheduleFixedRate(name, conf.FixedRate, runner)
	} else if conf.FixedDelay != "" {
		scheduleFixedDelay(name, conf.FixedDelay, runner)
	}
}

func scheduleCron(name, spec string, fn JobFunc) {
	_, err := cronManager.AddFunc(spec, func() {
		executeWithRecover(name, fn)
	})
	if err != nil {
		log.Printf("[SprinGo Scheduler] ❌ Error scheduling cron job '%s': %v", name, err)
	} else {
		log.Printf("[SprinGo Scheduler] 🕒 Job '%s' scheduled with cron: %s", name, spec)
	}
}

func scheduleFixedRate(name, durationStr string, fn JobFunc) {
	d, err := time.ParseDuration(durationStr)
	if err != nil {
		log.Printf("[SprinGo Scheduler] ❌ Invalid duration for fixed-rate job '%s': %v", name, err)
		return
	}

	schedulerWg.Add(1)
	go func() {
		defer schedulerWg.Done()
		runFixedRateLoop(name, d, fn, schedulerStopChan)
	}()
	log.Printf("[SprinGo Scheduler] 🕒 Job '%s' scheduled with fixed-rate: %s", name, durationStr)
}

func runFixedRateLoop(name string, d time.Duration, fn JobFunc, stopChan chan struct{}) {
	ticker := time.NewTicker(d)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			executeWithRecover(name, fn)
		case <-stopChan:
			log.Printf("[SprinGo Scheduler] Stopping fixed-rate loop for job '%s'", name)
			return
		}
	}
}

func scheduleFixedDelay(name, durationStr string, fn JobFunc) {
	d, err := time.ParseDuration(durationStr)
	if err != nil {
		log.Printf("[SprinGo Scheduler] ❌ Invalid duration for fixed-delay job '%s': %v", name, err)
		return
	}

	schedulerWg.Add(1)
	go func() {
		defer schedulerWg.Done()
		runFixedDelayLoop(name, d, fn, schedulerStopChan)
	}()
	log.Printf("[SprinGo Scheduler] 🕒 Job '%s' scheduled with fixed-delay: %s", name, durationStr)
}

func runFixedDelayLoop(name string, d time.Duration, fn JobFunc, stopChan chan struct{}) {
	for {
		executeWithRecover(name, fn)
		select {
		case <-time.After(d):
			// Proceed to next iteration
		case <-stopChan:
			log.Printf("[SprinGo Scheduler] Stopping fixed-delay loop for job '%s'", name)
			return
		}
	}
}

// StopBackgroundJobs cleanly stops the scheduler and waits for all active jobs to complete.
func StopBackgroundJobs() {
	schedulerMu.Lock()
	if !schedulerRunning {
		schedulerMu.Unlock()
		return
	}
	log.Println("[SprinGo Scheduler] Stopping background jobs engine...")

	if cronManager != nil {
		cronManager.Stop()
		cronManager = nil
	}

	if schedulerStopChan != nil {
		close(schedulerStopChan)
	}

	schedulerRunning = false
	schedulerMu.Unlock()

	// Wait for goroutines of fixed-rate and fixed-delay tasks to stop gracefully
	schedulerWg.Wait()
	schedulerStopChan = nil
	log.Println("[SprinGo Scheduler] ✅ Background jobs engine stopped gracefully")
}

func executeWithLock(name string, conf JobConf, fn JobFunc) error {
	db := ioc.GetContainer().GetDB()
	if db == nil {
		log.Printf("[SprinGo Scheduler] ⚠️ Lock enabled for job '%s' but primary database is not configured. Running without lock.", name)
		return executeWithRecover(name, fn)
	}

	atMost, atLeast := parseLockDurations(conf)
	now := time.Now()
	lockUntil := now.Add(atMost)

	acquired, err := acquireShedLock(db, name, lockUntil, now)
	if err != nil {
		log.Printf("[SprinGo Scheduler] ❌ Lock transaction failed for job '%s': %v", name, err)
		return err
	}

	if !acquired {
		return nil
	}

	log.Printf("[SprinGo Scheduler] 🔒 Lock ACQUIRED for job '%s' until %v (Instance: %s)", name, lockUntil, instanceID)

	jobErr := executeWithRecover(name, fn)

	releaseShedLock(db, name, now, atLeast)

	return jobErr
}

func parseLockDurations(conf JobConf) (time.Duration, time.Duration) {
	atMost := parseDurationWithFallback(conf.Lock.LockAtMostFor, 10*time.Minute)
	atLeast := parseDurationWithFallback(conf.Lock.LockAtLeastFor, 0)
	return atMost, atLeast
}

func parseDurationWithFallback(durationStr string, fallback time.Duration) time.Duration {
	d, err := time.ParseDuration(durationStr)
	if err != nil || d < 0 {
		return fallback
	}
	return d
}

func acquireShedLock(db *gorm.DB, name string, lockUntil time.Time, now time.Time) (bool, error) {
	helper := &acquireTxHelper{
		name:       name,
		lockUntil:  lockUntil,
		now:        now,
		instanceID: instanceID,
		lockRecord: ShedLockEntity{
			Name:      name,
			LockUntil: lockUntil,
			LockedAt:  now,
			LockedBy:  instanceID,
		},
	}
	err := db.Transaction(helper.execute)
	return helper.acquired, err
}

type acquireTxHelper struct {
	name       string
	lockUntil  time.Time
	now        time.Time
	instanceID string
	lockRecord ShedLockEntity
	acquired   bool
}

func (h *acquireTxHelper) execute(tx *gorm.DB) error {
	var existing ShedLockEntity
	if err := tx.Where("name = ?", h.name).First(&existing).Error; err != nil {
		return h.handleNotFound(tx, err)
	}
	return h.handleExisting(tx, existing)
}

func (h *acquireTxHelper) handleNotFound(tx *gorm.DB, err error) error {
	if err == gorm.ErrRecordNotFound {
		if createErr := tx.Create(&h.lockRecord).Error; createErr == nil {
			h.acquired = true
		}
		return nil
	}
	return err
}

func (h *acquireTxHelper) handleExisting(tx *gorm.DB, existing ShedLockEntity) error {
	if existing.LockUntil.Before(h.now) || existing.LockUntil.Equal(h.now) {
		return h.updateExpiredLock(tx)
	}
	return nil
}

func (h *acquireTxHelper) updateExpiredLock(tx *gorm.DB) error {
	result := tx.Model(&ShedLockEntity{}).
		Where("name = ? AND lock_until <= ?", h.name, h.now).
		Updates(map[string]interface{}{
			"lock_until": h.lockUntil,
			"locked_at":  h.now,
			"locked_by":  h.instanceID,
		})
	if result.Error == nil && result.RowsAffected > 0 {
		h.acquired = true
	}
	return nil
}

func releaseShedLock(db *gorm.DB, name string, now time.Time, atLeast time.Duration) {
	releaseTime := calculateReleaseTime(now, atLeast)

	if err := db.Model(&ShedLockEntity{}).Where("name = ? AND locked_by = ?", name, instanceID).Update("lock_until", releaseTime).Error; err != nil {
		log.Printf("[SprinGo Scheduler] ❌ Failed to release lock for job '%s': %v", name, err)
	} else {
		log.Printf("[SprinGo Scheduler] 🔓 Lock RELEASED for job '%s' (Hold until: %v)", name, releaseTime)
	}
}

func calculateReleaseTime(now time.Time, atLeast time.Duration) time.Time {
	releaseTime := time.Now()
	minReleaseTime := now.Add(atLeast)
	if releaseTime.Before(minReleaseTime) {
		return minReleaseTime
	}
	return releaseTime
}

type jobExecutionStats struct {
	LastExecuted time.Time `json:"last_executed,omitempty"`
	NextExpected time.Time `json:"next_expected,omitempty"`
}

var (
	statsMu        sync.RWMutex
	executionStats = make(map[string]*jobExecutionStats)
)

func trackExecution(name string) {
	statsMu.Lock()
	defer statsMu.Unlock()
	stats, exists := executionStats[name]
	if !exists {
		stats = &jobExecutionStats{}
		executionStats[name] = stats
	}
	stats.LastExecuted = time.Now()

	updateNextExpected(stats, name)
}

func updateNextExpected(stats *jobExecutionStats, name string) {
	props := config.Get[SchedulerProperties]()
	if props == nil {
		return
	}
	conf, ok := props.Jobs[name]
	if !ok {
		return
	}

	if conf.FixedRate != "" {
		if d, err := time.ParseDuration(conf.FixedRate); err == nil {
			stats.NextExpected = stats.LastExecuted.Add(d)
		}
	} else if conf.FixedDelay != "" {
		if d, err := time.ParseDuration(conf.FixedDelay); err == nil {
			stats.NextExpected = stats.LastExecuted.Add(d)
		}
	} else if conf.Cron != "" {
		parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
		if sched, err := parser.Parse(conf.Cron); err == nil {
			stats.NextExpected = sched.Next(stats.LastExecuted)
		}
	}
}

func executeWithRecover(name string, fn JobFunc) (err error) {
	trackExecution(name)
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[SprinGo Scheduler] 🚨 Panic recovered in job '%s': %v", name, r)
			err = fmt.Errorf("panic: %v", r)
		}
	}()

	return fn(context.Background())
}

// TriggerJobManually executes a registered job immediately and asynchronously in a separate goroutine
func TriggerJobManually(name string) error {
	mu.RLock()
	task, exists := registeredTasks[name]
	mu.RUnlock()

	if !exists {
		return fmt.Errorf("job '%s' is not registered in the system", name)
	}

	go func() {
		log.Printf("[SprinGo Scheduler] ⚡ Manual execution TRIGGERED for job '%s'", name)
		_ = executeWithRecover(name, task.fn)
		log.Printf("[SprinGo Scheduler] ⚡ Manual execution COMPLETED for job '%s'", name)
	}()

	return nil
}

// SchedulerJobInfo contains details of a registered job for the dashboard
type SchedulerJobInfo struct {
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
	LastExecuted time.Time `json:"last_executed,omitempty"`
	NextExpected time.Time `json:"next_expected,omitempty"`
}

// GetSchedulerJobs returns metadata about all configured and registered scheduler tasks
func GetSchedulerJobs() []SchedulerJobInfo {
	props := config.Get[SchedulerProperties]()
	if props == nil {
		return nil
	}

	mu.RLock()
	defer mu.RUnlock()

	var list []SchedulerJobInfo
	for name, conf := range props.Jobs {
		_, registered := registeredTasks[name]

		statsMu.RLock()
		stats, hasStats := executionStats[name]
		var lastEx, nextEx time.Time
		if hasStats {
			lastEx = stats.LastExecuted
			nextEx = stats.NextExpected
		}
		statsMu.RUnlock()

		list = append(list, SchedulerJobInfo{
			Name:         name,
			Cron:         conf.Cron,
			FixedRate:    conf.FixedRate,
			FixedDelay:   conf.FixedDelay,
			RunOnStartup: conf.RunOnStartup,
			Priority:     conf.Priority,
			Critical:     conf.Critical,
			Enabled:      conf.Enabled,
			LockEnabled:  conf.Lock.Enabled,
			Registered:   registered,
			LastExecuted: lastEx,
			NextExpected: nextEx,
		})
	}
	return list
}
