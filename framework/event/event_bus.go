package event

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/NeftaliAcosta/springo/framework/config"
	"github.com/NeftaliAcosta/springo/framework/database"
	"github.com/NeftaliAcosta/springo/framework/ioc"
	"github.com/NeftaliAcosta/springo/framework/web"
	"log"
	"os"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// EventListener is a function that reacts to a domain event
type EventListener func(ctx context.Context, event interface{}) error

var (
	listeners   = make(map[reflect.Type][]EventListener)
	listenersMu sync.RWMutex
)

// RegisterListener registers a new handler for a specific event type.
func RegisterListener(handler interface{}) {
	handlerVal := reflect.ValueOf(handler)
	if handlerVal.Kind() != reflect.Func {
		panic("event listener must be a function")
	}

	handlerType := handlerVal.Type()
	if handlerType.NumIn() != 2 {
		panic("event listener must have 2 arguments: context.Context and EventStruct")
	}

	// 1. Architecture Enforcement
	_, file, _, _ := runtime.Caller(1)
	if !strings.Contains(file, "internal/infrastructure/input/events") && !strings.Contains(file, "framework/event") {
		log.Printf("❌ [Architecture Error] Event listener MUST reside in 'internal/infrastructure/input/events/'. Found at: %s", file)
	}

	eventType := handlerType.In(1)

	wrappedHandler := func(ctx context.Context, event interface{}) error {
		args := []reflect.Value{
			reflect.ValueOf(ctx),
			reflect.ValueOf(event),
		}
		results := handlerVal.Call(args)
		if len(results) > 0 && !results[0].IsNil() {
			return results[0].Interface().(error)
		}
		return nil
	}

	listenersMu.Lock()
	defer listenersMu.Unlock()
	listeners[eventType] = append(listeners[eventType], wrappedHandler)
}

// EventPublisher is the interface for triggering events
type EventPublisher interface {
	Publish(ctx context.Context, event interface{})
}

type defaultEventPublisher struct{}

func (p *defaultEventPublisher) Publish(ctx context.Context, event interface{}) {
	props := config.Get[EventProperties]()
	if props == nil || !props.Enabled {
		return
	}

	tx := database.GetTxFromContext(ctx)
	var outboxID uint
	if tx != nil && props.Outbox.Enabled {
		payload, _ := json.Marshal(event)
		outboxEvent := OutboxEventEntity{
			EventName: reflect.TypeOf(event).String(),
			Payload:   string(payload),
			Status:    "PENDING",
			TraceID:   web.GetTraceID(ctx),
			CreatedAt: time.Now(),
		}
		if err := tx.Create(&outboxEvent).Error; err == nil {
			outboxID = outboxEvent.ID
		} else {
			log.Printf("❌ [Outbox] Failed to save outbox event: %v", err)
		}
	}

	txEvent := database.TransactionalEvent{
		Event:    event,
		OutboxID: outboxID,
	}

	// Transactional Support: If we are inside a TX, buffer the event
	if database.AddEventToTransaction(ctx, txEvent) {
		return // Event will be published after commit
	}

	p.dispatch(ctx, event, 0)
}

// eventTask represents a unit of work for event listeners to be executed in the pool.
type eventTask struct {
	ctx      context.Context
	listener EventListener
	event    interface{}
	outboxID uint
}

type workerPool struct {
	queue      chan eventTask
	done       chan struct{}
	workers    sync.WaitGroup
	publishers sync.WaitGroup
	fallbacks  sync.WaitGroup
	mu         sync.RWMutex
	accepting  bool
}

var (
	currentPool     *workerPool
	poolMu          sync.RWMutex
	rejPolicy       string
	discardedEvents int64
	poolStopped     bool
)

// GetDiscardedEventsCount returns the number of events discarded under the discard policy
func GetDiscardedEventsCount() int64 {
	return atomic.LoadInt64(&discardedEvents)
}

// ResetDiscardedEventsCount resets the discarded events counter to zero
func ResetDiscardedEventsCount() {
	atomic.StoreInt64(&discardedEvents, 0)
}

// StartWorkerPool initializes the background workers for asynchronous event processing.
// It complies with the configured pool size, queue capacity, and rejection policy.
func StartWorkerPool(props *EventProperties) {
	if props == nil || !props.Concurrency.Enabled {
		return
	}

	poolMu.Lock()
	defer poolMu.Unlock()

	// Prevent starting multiple pools (e.g. during test restarts)
	if currentPool != nil {
		return
	}

	size := props.Concurrency.PoolSize
	if size <= 0 {
		size = runtime.NumCPU() * 2
	}

	cap := props.Concurrency.QueueCapacity
	if cap <= 0 {
		cap = 100
	}

	policy := strings.ToLower(props.Concurrency.RejectionPolicy)
	if policy == "" {
		policy = "block"
	}
	rejPolicy = policy

	pool := &workerPool{
		queue:     make(chan eventTask, cap),
		done:      make(chan struct{}),
		accepting: true,
	}
	currentPool = pool
	poolStopped = false

	log.Printf("[EventBus] Starting worker pool with %d workers (Queue capacity: %d, Rejection policy: %s)", size, cap, policy)

	pool.workers.Add(size)
	for i := 0; i < size; i++ {
		go func(q chan eventTask) {
			defer pool.workers.Done()
			for task := range q {
				if err := task.listener(task.ctx, task.event); err != nil {
					log.Printf("⚠️ [EventBus] Error in listener for %T: %v", task.event, err)
					handleFailure(task.ctx, task.event, err)
				}
				finishListener(task.outboxID)
			}
		}(pool.queue)
	}

	if props.Outbox.Enabled {
		StartOutboxPoller(props)
	}
}

// StopWorkerPool cleanly stops the worker pool and releases resources.
func StopWorkerPool() {
	poolMu.Lock()
	pool := currentPool
	// From this point onward, publications that did not already register as
	// in-flight are rejected synchronously. This prevents unowned goroutines
	// from being created while the captured pool is draining.
	poolStopped = true
	if pool == nil {
		poolMu.Unlock()
		return
	}
	currentPool = nil
	poolMu.Unlock()

	pool.mu.Lock()
	pool.accepting = false
	pool.mu.Unlock()

	stopOutboxPoller()

	// Wait for all registered active publishers to finish pushing tasks to the queue or choosing a fallback
	pool.publishers.Wait()

	// Close the queue so workers know no new work is coming
	close(pool.queue)
	// Close done channel just in case (for compatibility)
	close(pool.done)

	// Wait for workers to finish draining the queue and exit
	pool.workers.Wait()

	// Wait for all fallback goroutines to finish
	pool.fallbacks.Wait()
}

var (
	outboxCounters   = make(map[uint]*int32)
	outboxCountersMu sync.Mutex
	outboxStopChan   chan struct{}
	outboxWg         sync.WaitGroup
)

func (p *defaultEventPublisher) dispatch(ctx context.Context, event interface{}, outboxID uint) {
	eventType := reflect.TypeOf(event)
	listenersMu.RLock()
	handlers, ok := listeners[eventType]
	listenersMu.RUnlock()

	if !ok {
		completeOutboxEvent(outboxID)
		return
	}

	numHandlers := len(handlers)
	if outboxID > 0 {
		outboxCountersMu.Lock()
		var count = int32(numHandlers)
		outboxCounters[outboxID] = &count
		outboxCountersMu.Unlock()
	}

	for _, handler := range handlers {
		p.dispatchToHandler(ctx, handler, event, outboxID)
	}
}

func (p *defaultEventPublisher) dispatchToHandler(ctx context.Context, handler EventListener, event interface{}, outboxID uint) {
	poolMu.RLock()
	pool := currentPool
	stopped := poolStopped
	if pool != nil {
		pool.publishers.Add(1)
	}
	policy := rejPolicy
	poolMu.RUnlock()

	if pool != nil {
		p.dispatchWithPool(pool, policy, ctx, handler, event, outboxID)
	} else if stopped {
		// A stopped pool must not create work without a lifecycle owner. Treat
		// the publication as an explicit rejection/discard so shutdown can
		// guarantee that no listener remains active after it returns.
		atomic.AddInt64(&discardedEvents, 1)
		finishListener(outboxID)
	} else {
		// Preserve the legacy behavior when concurrency was never started.
		go func(h EventListener, ev interface{}, oid uint) {
			if err := h(ctx, ev); err != nil {
				log.Printf("⚠️ [EventBus] Error in listener for %T: %v", ev, err)
				handleFailure(ctx, ev, err)
			}
			finishListener(oid)
		}(handler, event, outboxID)
	}
}

func (p *defaultEventPublisher) dispatchWithPool(pool *workerPool, policy string, ctx context.Context, handler EventListener, event interface{}, outboxID uint) {
	defer pool.publishers.Done()

	pool.mu.Lock()
	if pool.accepting {
		pool.mu.Unlock()
		p.dispatchAcceptingPool(pool, policy, ctx, handler, event, outboxID)
	} else {
		p.dispatchStoppingPool(pool, policy, ctx, handler, event, outboxID)
	}
}

func (p *defaultEventPublisher) dispatchStoppingPool(pool *workerPool, policy string, ctx context.Context, handler EventListener, event interface{}, outboxID uint) {
	// Enter with pool.mu locked
	if policy == "discard" {
		pool.mu.Unlock()
		finishListener(outboxID)
		atomic.AddInt64(&discardedEvents, 1)
	} else {
		pool.fallbacks.Add(1)
		pool.mu.Unlock()
		if os.Getenv("SPRINGO_PROFILES_ACTIVE") != "test" {
			log.Printf("⚠️ [EventBus] Pool stopping. Falling back to temporary goroutine for %T", event)
		}
		p.executeFallbackGoroutine(pool, ctx, handler, event, outboxID)
	}
}

func (p *defaultEventPublisher) dispatchAcceptingPool(pool *workerPool, policy string, ctx context.Context, handler EventListener, event interface{}, outboxID uint) {
	task := eventTask{ctx: ctx, listener: handler, event: event, outboxID: outboxID}
	sent := false

	switch policy {
	case "fallback":
		select {
		case pool.queue <- task:
			sent = true
		default:
			// Will execute fallback below
		}
	case "discard":
		select {
		case pool.queue <- task:
			sent = true
		default:
			log.Printf("⚠️ [EventBus] Queue full. Discarding event %T under discard policy", event)
			finishListener(outboxID)
			atomic.AddInt64(&discardedEvents, 1)
			sent = true // Treated as handled/discarded
		}
	case "block":
		// Since pool.queue is closed only after publishers.Wait() returns,
		// and we are currently an active publisher (counted in publishers.Add(1)),
		// this send is completely safe and cannot panic or block forever without workers draining.
		pool.queue <- task
		sent = true
	}

	if !sent {
		p.handleUnsentTask(pool, policy, ctx, handler, event, outboxID)
	}
}

func (p *defaultEventPublisher) handleUnsentTask(pool *workerPool, policy string, ctx context.Context, handler EventListener, event interface{}, outboxID uint) {
	if policy == "discard" {
		finishListener(outboxID)
		atomic.AddInt64(&discardedEvents, 1)
	} else {
		if os.Getenv("SPRINGO_PROFILES_ACTIVE") != "test" {
			log.Printf("⚠️ [EventBus] Queue full. Falling back to temporary goroutine for %T", event)
		}
		pool.mu.Lock()
		pool.fallbacks.Add(1)
		pool.mu.Unlock()
		p.executeFallbackGoroutine(pool, ctx, handler, event, outboxID)
	}
}

func (p *defaultEventPublisher) executeFallbackGoroutine(pool *workerPool, ctx context.Context, handler EventListener, event interface{}, outboxID uint) {
	go func(h EventListener, ev interface{}, oid uint) {
		defer pool.fallbacks.Done()
		if err := h(ctx, ev); err != nil {
			log.Printf("⚠️ [EventBus] Error in listener for %T: %v", ev, err)
			handleFailure(ctx, ev, err)
		}
		finishListener(oid)
	}(handler, event, outboxID)
}

func finishListener(outboxID uint) {
	if outboxID == 0 {
		return
	}

	outboxCountersMu.Lock()
	pCount, exists := outboxCounters[outboxID]
	outboxCountersMu.Unlock()

	if exists {
		newVal := atomic.AddInt32(pCount, -1)
		if newVal <= 0 {
			outboxCountersMu.Lock()
			delete(outboxCounters, outboxID)
			outboxCountersMu.Unlock()
			completeOutboxEvent(outboxID)
		}
	}
}

func completeOutboxEvent(outboxID uint) {
	if outboxID == 0 {
		return
	}

	props := config.Get[EventProperties]()
	if props == nil || !props.Outbox.Enabled {
		return
	}

	db := ioc.GetContainer().GetDB()
	if db == nil {
		return
	}

	if props.Outbox.CleanUp {
		if err := db.Delete(&OutboxEventEntity{}, outboxID).Error; err != nil {
			log.Printf("❌ [Outbox] Failed to delete completed outbox event %d: %v", outboxID, err)
		}
	} else {
		if err := db.Model(&OutboxEventEntity{}).Where("id = ?", outboxID).Update("status", "PROCESSED").Error; err != nil {
			log.Printf("❌ [Outbox] Failed to mark outbox event %d as PROCESSED: %v", outboxID, err)
		}
	}
}

func StartOutboxPoller(props *EventProperties) {
	if props == nil || !props.Outbox.Enabled {
		return
	}

	outboxStopChan = make(chan struct{})
	interval := 30 * time.Second
	if props.Outbox.PollInterval != "" {
		if d, err := time.ParseDuration(props.Outbox.PollInterval); err == nil {
			interval = d
		}
	}

	outboxWg.Add(1)
	go func() {
		defer outboxWg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		log.Printf("[Outbox] Starting background poller with interval %v", interval)

		for {
			select {
			case <-ticker.C:
				pollPendingEvents()
			case <-outboxStopChan:
				log.Println("[Outbox] Stopping background poller")
				return
			}
		}
	}()
}

func stopOutboxPoller() {
	if outboxStopChan != nil {
		close(outboxStopChan)
		outboxWg.Wait()
		outboxStopChan = nil
	}
}

func StopOutboxPoller() {
	poolMu.Lock()
	defer poolMu.Unlock()
	stopOutboxPoller()
}

func pollPendingEvents() {
	db := ioc.GetContainer().GetDB()
	if db == nil {
		return
	}

	recoverStuckEvents(db)

	// 2. Acquire cluster-wide poller lock
	acquired, err := acquirePollerLock(db)
	if err != nil {
		log.Printf("❌ [Outbox Poller] Lock query error: %v", err)
		return
	}
	if !acquired {
		return // Lock occupied by another replica, skip
	}
	defer releasePollerLock(db)

	pendingEvents, err := fetchPendingEvents(db)
	if err != nil || len(pendingEvents) == 0 {
		return
	}

	if err := claimEvents(db, pendingEvents); err != nil {
		log.Printf("❌ [Outbox Poller] Failed to claim processing status for events: %v", err)
		return
	}

	publisher := GetPublisher().(*defaultEventPublisher)
	dispatchPendingEvents(publisher, pendingEvents)
}

func recoverStuckEvents(db *gorm.DB) {
	// 1. Recover events stuck in PROCESSING for more than 5 minutes (in case of previous node crash)
	stuckTime := time.Now().Add(-5 * time.Minute)
	db.Model(&OutboxEventEntity{}).
		Where("status = ? AND updated_at < ?", "PROCESSING", stuckTime).
		Update("status", "PENDING")
}

func fetchPendingEvents(db *gorm.DB) ([]OutboxEventEntity, error) {
	cutoff := time.Now().Add(-5 * time.Second)
	var pendingEvents []OutboxEventEntity
	if err := db.Where("status = ? AND created_at < ?", "PENDING", cutoff).Limit(100).Find(&pendingEvents).Error; err != nil {
		log.Printf("❌ [Outbox Poller] Failed to fetch pending events: %v", err)
		return nil, err
	}

	if len(pendingEvents) > 0 {
		log.Printf("[Outbox Poller] Found %d pending events to process", len(pendingEvents))
	}
	return pendingEvents, nil
}

func claimEvents(db *gorm.DB, pendingEvents []OutboxEventEntity) error {
	// 3. Mark events as PROCESSING immediately to claim them and prevent double execution
	var eventIDs []uint
	for _, pe := range pendingEvents {
		eventIDs = append(eventIDs, pe.ID)
	}
	return db.Model(&OutboxEventEntity{}).Where("id IN ?", eventIDs).Update("status", "PROCESSING").Error
}

func dispatchPendingEvents(publisher *defaultEventPublisher, pendingEvents []OutboxEventEntity) {
	for _, pe := range pendingEvents {
		dispatchSinglePendingEvent(publisher, pe)
	}
}

func dispatchSinglePendingEvent(publisher *defaultEventPublisher, pe OutboxEventEntity) {
	typ := findEventTypeByName(pe.EventName)
	if typ == nil {
		log.Printf("⚠️ [Outbox Poller] Unknown event type '%s', skipping", pe.EventName)
		return
	}

	eventVal := reflect.New(typ).Interface()
	if err := json.Unmarshal([]byte(pe.Payload), eventVal); err != nil {
		log.Printf("❌ [Outbox Poller] Failed to unmarshal event %d payload: %v", pe.ID, err)
		return
	}

	var finalEvent interface{}
	if typ.Kind() == reflect.Pointer {
		finalEvent = eventVal
	} else {
		finalEvent = reflect.ValueOf(eventVal).Elem().Interface()
	}

	log.Printf("[Outbox Poller] Redespatching outbox event %d (%s)", pe.ID, pe.EventName)

	ctx := context.Background()
	if pe.TraceID != "" {
		ctx = web.WithTraceID(ctx, pe.TraceID)
	}

	publisher.dispatch(ctx, finalEvent, pe.ID)
}

func findEventTypeByName(name string) reflect.Type {
	listenersMu.RLock()
	defer listenersMu.RUnlock()
	for t := range listeners {
		if t.String() == name {
			return t
		}
	}
	return nil
}

func handleFailure(ctx context.Context, event interface{}, err error) {
	props := config.Get[EventProperties]()
	if props == nil || !props.DLQ.Enabled {
		return
	}

	db := ioc.GetContainer().GetDB()
	if db == nil {
		return
	}

	payload, _ := json.Marshal(event)
	failedEvent := FailedEventEntity{
		EventName:    reflect.TypeOf(event).String(),
		Payload:      string(payload),
		ListenerName: "DefaultListener",
		Error:        err.Error(),
		Status:       "PENDING",
		Retries:      0,
		TraceID:      web.GetTraceID(ctx),
		NextRetryAt:  CalculateNextRetry(0, props),
	}

	if err := db.Create(&failedEvent).Error; err != nil {
		log.Printf("❌ [DLQ] Failed to persist failed event: %v", err)
	} else {
		log.Printf("📥 [DLQ] Failed event saved to DB for retry: %s", failedEvent.EventName)
	}
}

// CalculateNextRetry determines the timestamp for the next attempt based on intervals
func CalculateNextRetry(attempts int, props *EventProperties) time.Time {
	duration := 30 * time.Second // Default fallback

	if props != nil && len(props.DLQ.RetryIntervals) > 0 {
		idx := attempts
		if idx >= len(props.DLQ.RetryIntervals) {
			idx = len(props.DLQ.RetryIntervals) - 1
		}

		if d, err := time.ParseDuration(props.DLQ.RetryIntervals[idx]); err == nil {
			duration = d
		}
	}

	return time.Now().Add(duration)
}

// GetPublisher returns the default event publisher
func GetPublisher() EventPublisher {
	return &defaultEventPublisher{}
}

// DispatchPendingEvents dispatches all events collected during a transaction
func DispatchPendingEvents(ctx context.Context, events []interface{}) {
	publisher := GetPublisher().(*defaultEventPublisher)
	for _, rawEvent := range events {
		if txEv, ok := rawEvent.(database.TransactionalEvent); ok {
			publisher.dispatch(ctx, txEv.Event, txEv.OutboxID)
		} else {
			publisher.dispatch(ctx, rawEvent, 0)
		}
	}
}

// PrintEventMap displays the current event routing table
func PrintEventMap() {
	listenersMu.RLock()
	defer listenersMu.RUnlock()

	if len(listeners) == 0 {
		return
	}

	log.Println("📢 [SprinGo EventBus] Event Routing Map:")
	for eventType, handlers := range listeners {
		log.Printf("   -> %v: %d listener(s)", eventType, len(handlers))
	}
}

var pollerInstanceID string

func init() {
	// Register the post-commit hook for transactional events
	database.RegisterPostCommitHook(DispatchPendingEvents)

	// Register DLQ retry callback in web layer to allow decoupled trigger from Actuator API
	web.RegisterDlqRetryCallback(RedispatchEvent)

	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		pollerInstanceID = "outbox-poller-unknown-" + uuid.New().String()
	} else {
		pollerInstanceID = "outbox-poller-" + hostname
	}
}

// OutboxPollerLockEntity is a local mapping of the ShedLock table to prevent package cycles.
type OutboxPollerLockEntity struct {
	Name      string    `gorm:"primaryKey;size:255"`
	LockUntil time.Time `gorm:"index"`
	LockedAt  time.Time
	LockedBy  string
}

// TableName matches the ShedLock table
func (OutboxPollerLockEntity) TableName() string {
	return "springo_shedlock"
}

func acquirePollerLock(db *gorm.DB) (bool, error) {
	now := time.Now()
	// Lock details: Poll runs every 30s, so we lock for 45s maximum to avoid deadlock
	lockDuration := 45 * time.Second
	lockUntil := now.Add(lockDuration)
	lockName := "outbox-poller-lock"

	lockRecord := OutboxPollerLockEntity{
		Name:      lockName,
		LockUntil: lockUntil,
		LockedAt:  now,
		LockedBy:  pollerInstanceID,
	}

	acquired := false
	err := db.Transaction(func(tx *gorm.DB) error {
		var existing OutboxPollerLockEntity
		err := tx.Where("name = ?", lockName).First(&existing).Error

		if err == gorm.ErrRecordNotFound {
			if err := tx.Create(&lockRecord).Error; err == nil {
				acquired = true
			}
			return nil
		}

		if err != nil {
			return err
		}

		if existing.LockUntil.Before(now) || existing.LockUntil.Equal(now) {
			result := tx.Model(&OutboxPollerLockEntity{}).
				Where("name = ? AND lock_until <= ?", lockName, now).
				Updates(map[string]interface{}{
					"lock_until": lockUntil,
					"locked_at":  now,
					"locked_by":  pollerInstanceID,
				})
			if result.Error == nil && result.RowsAffected > 0 {
				acquired = true
			}
		}
		return nil
	})

	return acquired, err
}

func releasePollerLock(db *gorm.DB) {
	lockName := "outbox-poller-lock"
	now := time.Now()
	if err := db.Model(&OutboxPollerLockEntity{}).
		Where("name = ? AND locked_by = ?", lockName, pollerInstanceID).
		Update("lock_until", now).Error; err != nil {
		log.Printf("❌ [Outbox Poller] Failed to release lock: %v", err)
	}
}

// RedispatchEvent deserializes a JSON payload to its registered event type and executes its listeners
func RedispatchEvent(ctx context.Context, eventName string, payload string) error {
	listenersMu.RLock()
	var foundType reflect.Type
	var handlers []EventListener
	for t, h := range listeners {
		tStr := t.String()
		tName := t.Name()
		if t.Kind() == reflect.Pointer {
			tName = t.Elem().Name()
		}

		cleanEventName := eventName
		if idx := strings.LastIndex(eventName, "."); idx != -1 {
			cleanEventName = eventName[idx+1:]
		}

		if tStr == eventName || tName == eventName || tName == cleanEventName ||
			strings.HasSuffix(tStr, "."+eventName) || strings.HasSuffix(eventName, "."+tName) ||
			strings.HasSuffix(tStr, "."+cleanEventName) {
			foundType = t
			handlers = h
			break
		}
	}
	listenersMu.RUnlock()

	if foundType == nil || len(handlers) == 0 {
		return fmt.Errorf("no listener registered for event type '%s'", eventName)
	}

	// Create a new instance pointer of the target event type
	ptr := reflect.New(foundType).Interface()
	if err := json.Unmarshal([]byte(payload), ptr); err != nil {
		return fmt.Errorf("failed to deserialize event payload: %w", err)
	}

	// Publish/dispatch the dereferenced event instance directly to handlers
	eventVal := reflect.ValueOf(ptr).Elem().Interface()

	var firstErr error
	for _, handler := range handlers {
		if err := handler(ctx, eventVal); err != nil {
			log.Printf("⚠️ [EventBus-Redispatch] Listener error for %s: %v", eventName, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}
