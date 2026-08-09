package scheduler

import (
	"context"
	"github.com/NeftaliAcosta/springo/framework/ioc"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestScheduler_ShedLock(t *testing.T) {
	// 1. Setup SQLite In-Memory Database
	db, err := gorm.Open(sqlite.Open("file:mem_shedlock?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)

	if err := db.AutoMigrate(&ShedLockEntity{}); err != nil {
		t.Fatalf("Failed to run ShedLock migrations: %v", err)
	}

	ioc.GetContainer().SetDB(db)
	defer ioc.GetContainer().Clear()

	// 2. Setup mock properties
	props := &SchedulerProperties{
		Enabled: true,
		Jobs: map[string]JobConf{
			"test-lock-job": {
				Enabled:      true,
				RunOnStartup: false,
				Lock: LockProperties{
					Enabled:        true,
					LockAtMostFor:  "1s",
					LockAtLeastFor: "500ms",
				},
			},
		},
	}
	ioc.GetContainer().RegisterBean("SchedulerProperties", props)

	// Clean/init registered tasks
	registeredTasks = make(map[string]taskDefinition)

	var runCount int
	var mu sync.Mutex

	Register("test-lock-job", func(ctx context.Context) error {
		mu.Lock()
		runCount++
		mu.Unlock()
		return nil
	})

	// 3. First execution: should acquire lock and execute
	err = executeWithLock("test-lock-job", props.Jobs["test-lock-job"], registeredTasks["test-lock-job"].fn)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	mu.Lock()
	firstCount := runCount
	mu.Unlock()

	if firstCount != 1 {
		t.Errorf("Expected job to execute once, got %d", firstCount)
	}

	// Check that lock record is in the DB
	var lock ShedLockEntity
	if err := db.First(&lock, "name = ?", "test-lock-job").Error; err != nil {
		t.Fatalf("Failed to find lock record in database: %v", err)
	}

	// Lock should hold until now + lock-at-least-for (500ms)
	if lock.LockUntil.Before(time.Now()) {
		t.Errorf("Lock should still be active under lock-at-least-for")
	}

	// 4. Second execution (immediate): since lock is active, running it again (even simulating another instance) should skip execution
	// Temporarily simulate a different instance to verify skip
	originalInstanceID := instanceID
	instanceID = "different-instance"
	defer func() { instanceID = originalInstanceID }()

	err = executeWithLock("test-lock-job", props.Jobs["test-lock-job"], registeredTasks["test-lock-job"].fn)
	if err != nil {
		t.Fatalf("Expected no error on skip, got %v", err)
	}

	mu.Lock()
	secondCount := runCount
	mu.Unlock()

	if secondCount != 1 {
		t.Errorf("Expected job to skip execution, but runCount increased to %d", secondCount)
	}

	// 5. Wait for lock to expire (atLeast: 500ms)
	time.Sleep(600 * time.Millisecond)

	// Now it should be able to execute again
	err = executeWithLock("test-lock-job", props.Jobs["test-lock-job"], registeredTasks["test-lock-job"].fn)
	if err != nil {
		t.Fatalf("Expected no error on rerun, got %v", err)
	}

	mu.Lock()
	thirdCount := runCount
	mu.Unlock()

	if thirdCount != 2 {
		t.Errorf("Expected job to execute again after lock expiration, got %d", thirdCount)
	}
}
