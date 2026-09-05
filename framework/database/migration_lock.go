package database

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/NeftaliAcosta/springo/framework/logging"
	"gorm.io/gorm"
)

var entropySource io.Reader = rand.Reader

// MigrationLock is the GORM model for the cluster-safe migration locking mechanism.
type MigrationLock struct {
	LockKey  string    `gorm:"primaryKey;size:50"`
	Locked   bool      `gorm:"not null"`
	LockedAt time.Time `gorm:"not null"`
	LockedBy string    `gorm:"size:255"`
}

// TableName returns the table name for MigrationLock.
func (MigrationLock) TableName() string {
	return "springo_migrations_lock"
}

func (m *MigrationManager) generateLockID() (string, error) {
	randBuf := make([]byte, 8)
	if _, err := io.ReadFull(entropySource, randBuf); err != nil {
		return "", fmt.Errorf("failed to generate unique lock token: %w", err)
	}
	uniqueToken := hex.EncodeToString(randBuf)
	if host, err := os.Hostname(); err == nil {
		return fmt.Sprintf("%s-%d-%s", host, os.Getpid(), uniqueToken), nil
	}
	return fmt.Sprintf("unknown-%d-%s", os.Getpid(), uniqueToken), nil
}

func (m *MigrationManager) waitForLock(db *gorm.DB, lockedBy string) error {
	maxWait := 2 * time.Minute
	pollInterval := 2 * time.Second
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		acquired, err := m.acquireLock(db, lockedBy)
		if err != nil {
			return fmt.Errorf("failed to acquire migration lock: %w", err)
		}
		if acquired {
			return nil
		}
		slog.Info(
			fmt.Sprintf("⏳ [Migrator] Database migration lock is currently held by another instance. Retrying in %v...",
				pollInterval),
			slog.String(logging.SubsystemKey, logging.FrameworkSubsystem),
			slog.Duration("retryInterval", pollInterval),
		)
		time.Sleep(pollInterval)
	}
	return fmt.Errorf("failed to acquire database migration lock after waiting %v", maxWait)
}

func (m *MigrationManager) startHeartbeat(db *gorm.DB, lockedBy string) (stop func()) {
	lockTimeout := 5 * time.Minute
	if props := m.getProps(); props != nil && props.GetMigrationLockTimeout() > 0 {
		lockTimeout = props.GetMigrationLockTimeout()
	}

	interval := lockTimeout / 3
	if interval > 30*time.Second {
		interval = 30 * time.Second
	}
	if interval < 100*time.Millisecond {
		interval = 100 * time.Millisecond
	}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.updateLockHeartbeat(db, lockedBy)
			}
		}
	}()

	return func() {
		cancel()
		wg.Wait()
		if err := m.releaseLock(db, lockedBy); err != nil {
			slog.Warn(fmt.Sprintf("⚠️ [Migrator] Failed to release database migration lock: %v", err),
				slog.String(logging.SubsystemKey, logging.FrameworkSubsystem),
				slog.Any("error", err))
		}
	}
}

func (m *MigrationManager) updateLockHeartbeat(db *gorm.DB, lockedBy string) {
	db.Table("springo_migrations_lock").
		Where("lock_key = ? AND locked_by = ?", "migration_lock", lockedBy).
		Update("locked_at", time.Now())
}

// acquireLock attempts to acquire the database migration lock.
func (m *MigrationManager) acquireLock(db *gorm.DB, lockedBy string) (bool, error) {
	if err := db.AutoMigrate(&MigrationLock{}); err != nil {
		return false, err
	}

	const lockKeyCondition = "lock_key = ?"

	// Ensure the lock record exists.
	var lock MigrationLock
	err := db.Where(lockKeyCondition, "migration_lock").First(&lock).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		lock = MigrationLock{
			LockKey:  "migration_lock",
			Locked:   false,
			LockedAt: time.Now(),
			LockedBy: "",
		}
		if err := db.Create(&lock).Error; err != nil {
			// Ignore parallel insert conflict and fetch again.
			db.Where(lockKeyCondition, "migration_lock").First(&lock)
		}
	}

	now := time.Now()
	timeout := 5 * time.Minute
	if props := m.getProps(); props != nil && props.GetMigrationLockTimeout() > 0 {
		timeout = props.GetMigrationLockTimeout()
	}

	// Try to acquire if unlocked.
	res := db.Model(&MigrationLock{}).
		Where("lock_key = ? AND locked = ?", "migration_lock", false).
		Updates(map[string]interface{}{
			"locked":    true,
			"locked_at": now,
			"locked_by": lockedBy,
		})

	if res.Error != nil {
		return false, res.Error
	}

	if res.RowsAffected > 0 {
		return true, nil
	}

	// Deadlock protection: break lock if it has expired.
	err = db.Where(lockKeyCondition, "migration_lock").First(&lock).Error
	if err != nil {
		return false, err
	}

	if lock.Locked && now.Sub(lock.LockedAt) > timeout {
		slog.Warn(fmt.Sprintf("⚠️ [Migrator] Breaking stale migration lock held by %s since %v",
			lock.LockedBy, lock.LockedAt),
			slog.String(logging.SubsystemKey, logging.FrameworkSubsystem),
			slog.String("lockedBy", lock.LockedBy),
			slog.Time("lockedAt", lock.LockedAt))
		res = db.Model(&MigrationLock{}).
			Where("lock_key = ? AND locked = ? AND locked_at = ?", "migration_lock", true, lock.LockedAt).
			Updates(map[string]interface{}{
				"locked":    true,
				"locked_at": now,
				"locked_by": lockedBy,
			})
		if res.Error != nil {
			return false, res.Error
		}
		if res.RowsAffected > 0 {
			return true, nil
		}
	}

	return false, nil
}

// releaseLock releases the acquired database migration lock.
func (m *MigrationManager) releaseLock(db *gorm.DB, lockedBy string) error {
	return db.Model(&MigrationLock{}).
		Where("lock_key = ? AND locked_by = ?", "migration_lock", lockedBy).
		Updates(map[string]interface{}{
			"locked":    false,
			"locked_by": "",
		}).Error
}

// refreshLock updates locked_at to keep the lock fresh during executions.
func (m *MigrationManager) refreshLock(db *gorm.DB, lockedBy string) {
	err := db.Model(&MigrationLock{}).
		Where("lock_key = ? AND locked_by = ? AND locked = ?", "migration_lock", lockedBy, true).
		Update("locked_at", time.Now()).Error
	if err != nil {
		slog.Warn(fmt.Sprintf("⚠️ [Migrator] Failed to refresh migration lock heartbeat: %v", err),
			slog.String(logging.SubsystemKey, logging.FrameworkSubsystem),
			slog.Any("error", err))
	}
}
