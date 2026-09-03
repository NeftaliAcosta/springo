package event

import (
	"context"
	"github.com/NeftaliAcosta/springo/framework/database"
	"github.com/NeftaliAcosta/springo/framework/ioc"
	"os"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func init() {
	_ = os.Setenv("SPRINGO_PROFILES_ACTIVE", "test")
}

type DummyEvent struct {
	ID string
}

func TestRegisterListener_Validation(t *testing.T) {
	t.Run("Panics on invalid listener function signature", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("Expected RegisterListener to panic on invalid function signature, but it did not")
			}
		}()
		RegisterListener("not-a-func")
	})
}

// TestEventBus_Unthrottled verifies that when concurrency is disabled,
// listeners are dispatched on separate, unthrottled goroutines.
func TestEventBus_Unthrottled(t *testing.T) {
	// Arrange
	listenersMu.Lock()
	listeners = make(map[reflect.Type][]EventListener) // Reset routing table
	listenersMu.Unlock()

	var wg sync.WaitGroup
	wg.Add(3)

	RegisterListener(func(ctx context.Context, ev DummyEvent) error {
		wg.Done()
		return nil
	})

	props := &EventProperties{
		Enabled: true,
	}
	ioc.GetContainer().RegisterBean("EventProperties", props)
	StartWorkerPool(props)
	defer StopWorkerPool()

	publisher := GetPublisher()

	// Act
	publisher.Publish(context.Background(), DummyEvent{ID: "1"})
	publisher.Publish(context.Background(), DummyEvent{ID: "2"})
	publisher.Publish(context.Background(), DummyEvent{ID: "3"})

	// Assert: should complete immediately as they run asynchronously
	wg.Wait()
}

// TestEventBus_WorkerPool verifies that when concurrency is enabled,
// events are successfully queued and executed by workers.
func TestEventBus_WorkerPool(t *testing.T) {
	// Arrange
	listenersMu.Lock()
	listeners = make(map[reflect.Type][]EventListener) // Reset routing table
	listenersMu.Unlock()

	var wg sync.WaitGroup
	wg.Add(3)

	RegisterListener(func(ctx context.Context, ev DummyEvent) error {
		wg.Done()
		return nil
	})

	props := &EventProperties{
		Enabled: true,
		Concurrency: ConcurrencyProperties{
			Enabled:         true,
			PoolSize:        2,
			QueueCapacity:   10,
			RejectionPolicy: "block",
		},
	}
	ioc.GetContainer().RegisterBean("EventProperties", props)

	StartWorkerPool(props)
	defer StopWorkerPool()

	publisher := GetPublisher()

	// Act
	publisher.Publish(context.Background(), DummyEvent{ID: "1"})
	publisher.Publish(context.Background(), DummyEvent{ID: "2"})
	publisher.Publish(context.Background(), DummyEvent{ID: "3"})

	// Assert
	wg.Wait()
}

// TestEventBus_RejectionPolicyFallback verifies that under "fallback" policy,
// if the queue and workers are saturated, tasks are executed on separate temporary goroutines.
func TestEventBus_RejectionPolicyFallback(t *testing.T) {
	// Arrange
	listenersMu.Lock()
	listeners = make(map[reflect.Type][]EventListener) // Reset routing table
	listenersMu.Unlock()

	var wg sync.WaitGroup
	wg.Add(4)

	blockChan := make(chan struct{}, 4)
	defer close(blockChan)

	startedChan := make(chan string, 4)

	RegisterListener(func(ctx context.Context, ev DummyEvent) error {
		startedChan <- ev.ID
		<-blockChan
		wg.Done()
		return nil
	})

	// 1 worker, 1 queue capacity. Capacity saturated at 2 items.
	props := &EventProperties{
		Enabled: true,
		Concurrency: ConcurrencyProperties{
			Enabled:         true,
			PoolSize:        1,
			QueueCapacity:   1,
			RejectionPolicy: "fallback",
		},
	}
	ioc.GetContainer().RegisterBean("EventProperties", props)

	StartWorkerPool(props)
	defer StopWorkerPool()

	publisher := GetPublisher()

	// Act
	publisher.Publish(context.Background(), DummyEvent{ID: "1"}) // Satures worker

	// Wait for task 1 to start executing so we know the worker is busy
	for val := range startedChan {
		if val == "1" {
			break
		}
	}

	publisher.Publish(context.Background(), DummyEvent{ID: "2"}) // Saturates queue

	// Since task 2 is in the queue, tasks 3 and 4 will hit the fallback path.
	// Spawning fallback goroutines is asynchronous, but we can wait until they start executing.
	publisher.Publish(context.Background(), DummyEvent{ID: "3"}) // Triggers fallback goroutine
	publisher.Publish(context.Background(), DummyEvent{ID: "4"}) // Triggers fallback goroutine

	// Wait for fallback tasks 3 and 4 to start executing
	var started3, started4 bool
	for !started3 || !started4 {
		id := <-startedChan
		switch id {
		case "3":
			started3 = true
		case "4":
			started4 = true
		}
	}

	// Unblock all workers and fallback routines
	blockChan <- struct{}{}
	blockChan <- struct{}{}
	blockChan <- struct{}{}
	blockChan <- struct{}{}

	// Assert: All tasks should be completed successfully
	wg.Wait()
}

// TestEventBus_RejectionPolicyDiscard verifies that under "discard" policy,
// tasks that exceed queue capacity are silently dropped.
func TestEventBus_RejectionPolicyDiscard(t *testing.T) {
	// Arrange
	listenersMu.Lock()
	listeners = make(map[reflect.Type][]EventListener) // Reset routing table
	listenersMu.Unlock()

	var runCount int
	var mu sync.Mutex

	blockChan := make(chan struct{}, 4)
	defer close(blockChan)

	startedChan := make(chan string, 4)

	RegisterListener(func(ctx context.Context, ev DummyEvent) error {
		startedChan <- ev.ID
		<-blockChan
		mu.Lock()
		runCount++
		mu.Unlock()
		return nil
	})

	// 1 worker, 1 queue capacity. Saturated at 2 items.
	props := &EventProperties{
		Enabled: true,
		Concurrency: ConcurrencyProperties{
			Enabled:         true,
			PoolSize:        1,
			QueueCapacity:   1,
			RejectionPolicy: "discard",
		},
	}
	ioc.GetContainer().RegisterBean("EventProperties", props)

	StartWorkerPool(props)
	defer StopWorkerPool()

	publisher := GetPublisher()

	// Act
	publisher.Publish(context.Background(), DummyEvent{ID: "1"}) // Satures worker

	// Wait for task 1 to start executing so we know the worker is busy
	for val := range startedChan {
		if val == "1" {
			break
		}
	}

	publisher.Publish(context.Background(), DummyEvent{ID: "2"}) // Saturates queue

	// Since task 2 is in the queue, tasks 3 and 4 will be discarded.
	publisher.Publish(context.Background(), DummyEvent{ID: "3"}) // Discarded
	publisher.Publish(context.Background(), DummyEvent{ID: "4"}) // Discarded

	// Unblock task 1 (the executing worker)
	blockChan <- struct{}{}

	// Wait for task 2 to start executing
	for val := range startedChan {
		if val == "2" {
			break
		}
	}

	// Unblock task 2
	blockChan <- struct{}{}

	// Wait briefly for async execution to finish setting runCount
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	finalCount := runCount
	mu.Unlock()

	// Assert: Only 2 tasks should have executed
	if finalCount != 2 {
		t.Errorf("Expected only 2 events to run under discard policy, got %d", finalCount)
	}
}

func TestEventBus_PhysicalOutbox(t *testing.T) {
	// 1. Setup in-memory sqlite database
	db, err := gorm.Open(sqlite.Open("file:mem_physical_outbox?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)

	// 2. Auto-migrate OutboxEventEntity and OutboxPollerLockEntity
	if err := db.AutoMigrate(&OutboxEventEntity{}, &OutboxPollerLockEntity{}); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// 3. Register DB in IoC Container
	ioc.GetContainer().SetDB(db)
	defer ioc.GetContainer().Clear()

	// 4. Setup Routing & Listeners
	listenersMu.Lock()
	listeners = make(map[reflect.Type][]EventListener)
	listenersMu.Unlock()

	var wg sync.WaitGroup
	wg.Add(1)

	RegisterListener(func(ctx context.Context, ev DummyEvent) error {
		wg.Done()
		return nil
	})

	// 5. Configure EventProperties with Outbox enabled
	props := &EventProperties{
		Enabled: true,
		Concurrency: ConcurrencyProperties{
			Enabled:         true,
			PoolSize:        2,
			QueueCapacity:   10,
			RejectionPolicy: "block",
		},
		Outbox: OutboxProperties{
			Enabled:      true,
			CleanUp:      false, // keep records for assertion
			PollInterval: "1s",
		},
	}
	ioc.GetContainer().RegisterBean("EventProperties", props)

	StartWorkerPool(props)
	defer StopWorkerPool()

	publisher := GetPublisher()

	// 6. Act & Assert: Inside Transactional context
	err = database.Transactional(context.Background(), func(ctx context.Context) error {
		// Publish event inside TX
		publisher.Publish(ctx, DummyEvent{ID: "outbox-test-1"})

		// Assert event is physically written to springo_outbox as PENDING
		tx := database.GetTxFromContext(ctx)
		if tx == nil {
			t.Errorf("Expected active transaction in context")
		}

		var count int64
		if err := tx.Model(&OutboxEventEntity{}).Where("status = ?", "PENDING").Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			t.Errorf("Expected 1 pending event inside TX, got %d", count)
		}

		return nil // Commit transaction
	})

	if err != nil {
		t.Fatalf("Transactional failed: %v", err)
	}

	// Wait for async execution of event listener (post-commit dispatch)
	wg.Wait()
	time.Sleep(50 * time.Millisecond)

	// Assert event status updated to PROCESSED in DB
	var outboxEvent OutboxEventEntity
	if err := db.First(&outboxEvent).Error; err != nil {
		t.Fatalf("Failed to fetch outbox event from db: %v", err)
	}

	if outboxEvent.Status != "PROCESSED" {
		t.Errorf("Expected outbox event to be PROCESSED, got: %s", outboxEvent.Status)
	}

	if outboxEvent.EventName != "event.DummyEvent" {
		t.Errorf("Expected event name 'event.DummyEvent', got '%s'", outboxEvent.EventName)
	}
}

func TestEventBus_OutboxPoller(t *testing.T) {
	// 1. Setup in-memory sqlite database
	db, err := gorm.Open(sqlite.Open("file:mem_outbox_poller?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)

	if err := db.AutoMigrate(&OutboxEventEntity{}, &OutboxPollerLockEntity{}); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	ioc.GetContainer().SetDB(db)
	defer ioc.GetContainer().Clear()

	listenersMu.Lock()
	listeners = make(map[reflect.Type][]EventListener)
	listenersMu.Unlock()

	var wg sync.WaitGroup
	wg.Add(1)

	var runID string
	var mu sync.Mutex

	RegisterListener(func(ctx context.Context, ev DummyEvent) error {
		mu.Lock()
		runID = ev.ID
		mu.Unlock()
		wg.Done()
		return nil
	})

	// Create a PENDING outbox event in the database, created 10 seconds ago so poller picks it up
	oldEvent := OutboxEventEntity{
		EventName: "event.DummyEvent",
		Payload:   `{"ID":"poller-orphan-1"}`,
		Status:    "PENDING",
		CreatedAt: time.Now().Add(-10 * time.Second),
	}
	if err := db.Create(&oldEvent).Error; err != nil {
		t.Fatalf("Failed to insert mock pending event: %v", err)
	}

	// Configure EventProperties with Outbox poller enabled
	props := &EventProperties{
		Enabled: true,
		Concurrency: ConcurrencyProperties{
			Enabled:         true,
			PoolSize:        2,
			QueueCapacity:   10,
			RejectionPolicy: "block",
		},
		Outbox: OutboxProperties{
			Enabled:      true,
			CleanUp:      true,    // this time cleanup completed events
			PollInterval: "100ms", // aggressive polling for test
		},
	}
	ioc.GetContainer().RegisterBean("EventProperties", props)

	// This starts the poller which will scan the database
	StartWorkerPool(props)
	defer StopWorkerPool()

	// Wait for the poller to pick it up, run it, and complete
	wg.Wait()
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	finalRunID := runID
	mu.Unlock()

	if finalRunID != "poller-orphan-1" {
		t.Errorf("Expected listener to run for 'poller-orphan-1', got '%s'", finalRunID)
	}

	// Verify the event was cleaned up (deleted) from the database
	var count int64
	db.Model(&OutboxEventEntity{}).Count(&count)
	if count != 0 {
		t.Errorf("Expected outbox event to be deleted due to cleanup: true, found %d records", count)
	}
}

func TestEventBus_LifecycleRaceSafety(t *testing.T) {
	listenersMu.Lock()
	listeners = make(map[reflect.Type][]EventListener)
	listenersMu.Unlock()

	RegisterListener(func(ctx context.Context, ev DummyEvent) error {
		time.Sleep(1 * time.Millisecond)
		return nil
	})

	props := &EventProperties{
		Enabled: true,
		Concurrency: ConcurrencyProperties{
			Enabled:         true,
			PoolSize:        4,
			QueueCapacity:   50,
			RejectionPolicy: "block",
		},
	}
	ioc.GetContainer().RegisterBean("EventProperties", props)

	var wg sync.WaitGroup
	publishStop := make(chan struct{})

	// Spawn concurrent publishers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			publisher := GetPublisher()
			for {
				select {
				case <-publishStop:
					return
				default:
					publisher.Publish(context.Background(), DummyEvent{ID: "race"})
					time.Sleep(10 * time.Microsecond)
				}
			}
		}()
	}

	// Spin Start & Stop worker pools repeatedly
	for cycle := 0; cycle < 10; cycle++ {
		StartWorkerPool(props)
		time.Sleep(5 * time.Millisecond)
		StopWorkerPool()
		time.Sleep(5 * time.Millisecond)
	}

	close(publishStop)
	wg.Wait()
}

func TestEventBus_SemanticAccounting(t *testing.T) {
	listenersMu.Lock()
	listeners = make(map[reflect.Type][]EventListener)
	listenersMu.Unlock()

	var processed int32

	RegisterListener(func(ctx context.Context, ev DummyEvent) error {
		atomic.AddInt32(&processed, 1)
		time.Sleep(100 * time.Microsecond)
		return nil
	})

	props := &EventProperties{
		Enabled: true,
		Concurrency: ConcurrencyProperties{
			Enabled:         true,
			PoolSize:        4,
			QueueCapacity:   50,
			RejectionPolicy: "block",
		},
	}
	ioc.GetContainer().RegisterBean("EventProperties", props)

	ResetDiscardedEventsCount()
	StartWorkerPool(props)

	var wg sync.WaitGroup
	var published int32
	stopPublishers := make(chan struct{})

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			publisher := GetPublisher()
			for {
				select {
				case <-stopPublishers:
					return
				default:
					atomic.AddInt32(&published, 1)
					publisher.Publish(context.Background(), DummyEvent{ID: "accounting"})
					time.Sleep(10 * time.Microsecond)
				}
			}
		}()
	}

	time.Sleep(10 * time.Millisecond)
	close(stopPublishers)
	wg.Wait() // Wait for all publishers to stop sending

	// Now call StopWorkerPool. It must drain and wait for all queued and fallback tasks
	StopWorkerPool()

	pub := atomic.LoadInt32(&published)
	proc := atomic.LoadInt32(&processed)
	disc := GetDiscardedEventsCount()

	if int64(pub) != int64(proc)+disc {
		t.Errorf("Expected published (%d) to equal processed (%d) + discarded (%d)", pub, proc, disc)
	}
}

func TestEventBus_PublishDuringShutdown(t *testing.T) {
	listenersMu.Lock()
	listeners = make(map[reflect.Type][]EventListener)
	listenersMu.Unlock()

	var processed int32
	RegisterListener(func(ctx context.Context, ev DummyEvent) error {
		atomic.AddInt32(&processed, 1)
		return nil
	})

	props := &EventProperties{
		Enabled: true,
		Concurrency: ConcurrencyProperties{
			Enabled:         true,
			PoolSize:        4,
			QueueCapacity:   20,
			RejectionPolicy: "fallback",
		},
	}
	ioc.GetContainer().RegisterBean("EventProperties", props)

	ResetDiscardedEventsCount()
	StartWorkerPool(props)

	var wg sync.WaitGroup
	var published int32
	stopPublishers := make(chan struct{})

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			publisher := GetPublisher()
			for {
				select {
				case <-stopPublishers:
					return
				default:
					atomic.AddInt32(&published, 1)
					publisher.Publish(context.Background(), DummyEvent{ID: "shutdown-test"})
				}
			}
		}()
	}

	time.Sleep(5 * time.Millisecond)

	// Stop pool concurrently while publishers are still active
	StopWorkerPool()

	close(stopPublishers)
	wg.Wait()

	pub := atomic.LoadInt32(&published)
	proc := atomic.LoadInt32(&processed)
	disc := GetDiscardedEventsCount()

	if int64(pub) != int64(proc)+disc {
		t.Fatalf("Published events were not fully accounted at shutdown: published=%d processed=%d discarded=%d", pub, proc, disc)
	}

	// Once StopWorkerPool returns, no listener may still be running.
	processedAtShutdown := atomic.LoadInt32(&processed)
	runtime.Gosched()
	if got := atomic.LoadInt32(&processed); got != processedAtShutdown {
		t.Fatalf("listener count changed after shutdown returned: before=%d after=%d", processedAtShutdown, got)
	}
}

func TestDLQ_ManualRetryAndProcessDLQ(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite in-memory db: %v", err)
	}
	if err := db.AutoMigrate(&FailedEventEntity{}); err != nil {
		t.Fatalf("failed to migrate DLQ table: %v", err)
	}

	ioc.GetContainer().RegisterBean("DB", db)

	listenersMu.Lock()
	listeners = make(map[reflect.Type][]EventListener)
	listenersMu.Unlock()

	var successCount int32
	RegisterListener(func(ctx context.Context, ev DummyEvent) error {
		atomic.AddInt32(&successCount, 1)
		return nil
	})

	props := &EventProperties{
		Enabled: true,
		DLQ: DLQProperties{
			Enabled:        true,
			MaxRetries:     3,
			RetryIntervals: []string{"1s", "2s"},
		},
	}
	ioc.GetContainer().RegisterBean("EventProperties", props)

	// 1. Create a failed event in DLQ directly
	failedEv := FailedEventEntity{
		EventName:    reflect.TypeOf(DummyEvent{}).String(),
		Payload:      `{"ID":"dlq-test-1"}`,
		ListenerName: "DefaultListener",
		Error:        "simulated failure",
		Status:       "FAILED",
		Retries:      0,
		NextRetryAt:  time.Now().Add(-1 * time.Minute),
	}
	if err := db.Create(&failedEv).Error; err != nil {
		t.Fatalf("failed to create failed event: %v", err)
	}

	// 2. Test manual retry via RedispatchEvent (as used by Actuator /dlq/retry)
	if err := RedispatchEvent(context.Background(), failedEv.EventName, failedEv.Payload); err != nil {
		t.Fatalf("RedispatchEvent failed: %v", err)
	}
	db.Delete(&FailedEventEntity{}, failedEv.ID)

	if atomic.LoadInt32(&successCount) != 1 {
		t.Fatalf("expected listener to be called 1 time, got %d", successCount)
	}

	// 3. Test ProcessDLQ automated scanning & recovery
	failedEv2 := FailedEventEntity{
		EventName:    reflect.TypeOf(DummyEvent{}).String(),
		Payload:      `{"ID":"dlq-test-2"}`,
		ListenerName: "DefaultListener",
		Error:        "simulated failure 2",
		Status:       "FAILED",
		Retries:      0,
		NextRetryAt:  time.Now().Add(-1 * time.Minute),
	}
	db.Create(&failedEv2)

	manager := &RetryManager{}
	if err := manager.ProcessDLQ(); err != nil {
		t.Fatalf("ProcessDLQ failed: %v", err)
	}

	if atomic.LoadInt32(&successCount) != 2 {
		t.Fatalf("expected listener to be called 2 times after ProcessDLQ, got %d", successCount)
	}

	var count int64
	db.Model(&FailedEventEntity{}).Where("id = ?", failedEv2.ID).Count(&count)
	if count != 0 {
		t.Fatalf("expected failed event %d to be deleted from DLQ after successful recovery, but it remains", failedEv2.ID)
	}
}
