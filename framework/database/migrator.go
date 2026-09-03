package database

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/NeftaliAcosta/springo/framework/config"
	"io"
	"log"
	"os"
	"regexp"
	"sort"
	"sync"
	"time"

	"gorm.io/gorm"
)

var entropySource io.Reader = rand.Reader

// Migration represents a single database change.
type Migration struct {
	Name     string
	Up       func(db *gorm.DB) error
	Down     func(db *gorm.DB) error
	Checksum string // Optional checksum to validate integrity of migration logic
}

// MigrationRecord is the GORM model for the control table.
type MigrationRecord struct {
	ID        uint      `gorm:"primaryKey"`
	Migration string    `gorm:"size:255;uniqueIndex"`
	Batch     int       `gorm:"not null"`
	Checksum  string    `gorm:"size:255"`
	CreatedAt time.Time `gorm:"not null"`
}

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

var (
	migrationsMu         sync.RWMutex
	registeredMigrations []Migration
	customTableName      = "springo_migrations" // Default table name matching SprinGo naming
)

// SetMigrationTableName allows customizing the control table name.
func SetMigrationTableName(name string) {
	if name != "" {
		migrationsMu.Lock()
		customTableName = name
		migrationsMu.Unlock()
	}
}

func getCustomTableName() string {
	migrationsMu.RLock()
	defer migrationsMu.RUnlock()
	return customTableName
}

// RegisterMigration adds a migration to the execution queue.
func RegisterMigration(m Migration) {
	migrationsMu.Lock()
	defer migrationsMu.Unlock()
	registeredMigrations = append(registeredMigrations, m)
}

func getRegisteredMigrationsSnapshot() []Migration {
	migrationsMu.RLock()
	defer migrationsMu.RUnlock()
	snapshot := make([]Migration, len(registeredMigrations))
	copy(snapshot, registeredMigrations)
	return snapshot
}

// MigrationManager handles the execution, validation, and lock-safety of database migrations.
type MigrationManager struct {
	db    *gorm.DB
	debug bool
}

// NewMigrationManager creates a new instance of MigrationManager.
func NewMigrationManager(db *gorm.DB, debug bool) *MigrationManager {
	return &MigrationManager{db: db, debug: debug}
}

// getDB returns the correct database session based on debug mode.
func (m *MigrationManager) getDB() *gorm.DB {
	if m.debug {
		return m.db
	}
	return m.db.Session(&gorm.Session{Logger: m.db.Logger.LogMode(1)})
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
		log.Printf(
			"⏳ [Migrator] Database migration lock is currently held by another instance. Retrying in %v...",
			pollInterval,
		)
		time.Sleep(pollInterval)
	}
	return fmt.Errorf("failed to acquire database migration lock after waiting %v", maxWait)
}

func (m *MigrationManager) startHeartbeat(db *gorm.DB, lockedBy string) (stop func()) {
	lockTimeout := 5 * time.Minute
	if props := config.Get[DataSourceProperties](); props != nil && props.MigrationLockTimeout > 0 {
		lockTimeout = props.MigrationLockTimeout
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
			log.Printf("⚠️ [Migrator] Failed to release database migration lock: %v", err)
		}
	}
}

func (m *MigrationManager) updateLockHeartbeat(db *gorm.DB, lockedBy string) {
	db.Table("springo_migrations_lock").
		Where("lock_key = ? AND locked_by = ?", "migration_lock", lockedBy).
		Update("locked_at", time.Now())
}

func (m *MigrationManager) getExecutedMigrationsMap(db *gorm.DB) (map[string]MigrationRecord, error) {
	if err := db.Table(getCustomTableName()).AutoMigrate(&MigrationRecord{}); err != nil {
		return nil, fmt.Errorf("failed to auto-migrate table %s: %w", getCustomTableName(), err)
	}
	var executed []MigrationRecord
	if err := db.Table(getCustomTableName()).Find(&executed).Error; err != nil {
		return nil, fmt.Errorf("failed to retrieve executed migrations: %w", err)
	}
	executedMap := make(map[string]MigrationRecord)
	for _, e := range executed {
		executedMap[e.Migration] = e
	}
	return executedMap, nil
}

func (m *MigrationManager) validateChecksums(executedMap map[string]MigrationRecord) error {
	for _, reg := range getRegisteredMigrationsSnapshot() {
		if record, ok := executedMap[reg.Name]; ok {
			expectedChecksum := computeChecksum(reg)
			if reg.Checksum != "" && record.Checksum != "" && expectedChecksum != record.Checksum {
				return fmt.Errorf(
					"integrity violation: migration %q has checksum %q, but database record has %q. "+
						"The migration logic has been modified after execution",
					reg.Name,
					expectedChecksum,
					record.Checksum,
				)
			}
		}
	}
	return nil
}

func (m *MigrationManager) getPendingMigrations(executedMap map[string]MigrationRecord) []Migration {
	var pending []Migration
	for _, reg := range getRegisteredMigrationsSnapshot() {
		if _, ok := executedMap[reg.Name]; !ok {
			pending = append(pending, reg)
		}
	}
	return pending
}

func (m *MigrationManager) getNextBatch(db *gorm.DB) int {
	var lastRecord MigrationRecord
	db.Table(getCustomTableName()).
		Session(&gorm.Session{Logger: db.Logger.LogMode(1)}).
		Order("batch desc").
		First(&lastRecord)
	return lastRecord.Batch + 1
}

func (m *MigrationManager) executePendingMigrations(pending []Migration, nextBatch int) error {
	log.Printf("🚀 Running migrations for batch %d...", nextBatch)
	for _, p := range pending {
		log.Printf("  -> Migrating: %s", p.Name)
		checksum := computeChecksum(p)
		err := m.db.Transaction(func(tx *gorm.DB) error {
			if err := p.Up(tx); err != nil {
				return err
			}
			return tx.Table(getCustomTableName()).Create(&MigrationRecord{
				Migration: p.Name,
				Batch:     nextBatch,
				Checksum:  checksum,
				CreatedAt: time.Now(),
			}).Error
		})

		if err != nil {
			log.Printf("  ❌ Error migrating %s: %v", p.Name, err)
			return err
		}
	}
	return nil
}

// Migrate executes all pending migrations sequentially using a cluster-safe database lock.
func (m *MigrationManager) Migrate() error {
	db := m.getDB()

	lockedBy, err := m.generateLockID()
	if err != nil {
		return err
	}

	if err := m.waitForLock(db, lockedBy); err != nil {
		return err
	}

	stop := m.startHeartbeat(db, lockedBy)
	defer stop()

	executedMap, err := m.getExecutedMigrationsMap(db)
	if err != nil {
		return err
	}

	if err := m.validateChecksums(executedMap); err != nil {
		return err
	}

	pending := m.getPendingMigrations(executedMap)
	if len(pending) == 0 {
		return nil
	}

	sort.Slice(pending, func(i, j int) bool {
		return compareMigrationNames(pending[i].Name, pending[j].Name)
	})

	if err := m.executePendingMigrations(pending, m.getNextBatch(db)); err != nil {
		return err
	}

	log.Printf("✅ Database migrations completed successfully.")
	return nil
}

// Rollback reverses the last 'steps' migrations (or the entire last batch if steps <= 0).
func (m *MigrationManager) Rollback(steps int) error {
	db := m.getDB()

	lockedBy, err := m.generateLockID()
	if err != nil {
		return err
	}

	if err := m.waitForLock(db, lockedBy); err != nil {
		return err
	}

	stop := m.startHeartbeat(db, lockedBy)
	defer stop()

	return m.rollbackUnderLock(db, steps)
}

func (m *MigrationManager) rollbackUnderLock(db *gorm.DB, steps int) error {
	executed, err := m.getExecutedMigrationsDescending(db)
	if err != nil {
		return err
	}

	if len(executed) == 0 {
		log.Println("ℹ️ No migrations found to rollback.")
		return nil
	}

	toRevert := m.getMigrationsToRevert(executed, steps)

	log.Printf("🔄 Rolling back %d migration(s)...", len(toRevert))

	return m.executeReversions(toRevert)
}

// getExecutedMigrationsDescending ensures the control table exists and returns
// its records in rollback order. Initializing the table keeps reset/refresh a
// no-op on a fresh database while still propagating real database failures.
func (m *MigrationManager) getExecutedMigrationsDescending(db *gorm.DB) ([]MigrationRecord, error) {
	tableName := getCustomTableName()
	if err := db.Table(tableName).AutoMigrate(&MigrationRecord{}); err != nil {
		return nil, fmt.Errorf("failed to initialize migrations table %q: %w", tableName, err)
	}

	var executed []MigrationRecord
	if err := db.Table(tableName).Order("id desc").Find(&executed).Error; err != nil {
		return nil, fmt.Errorf("failed to retrieve executed migrations for rollback: %w", err)
	}
	return executed, nil
}

func (m *MigrationManager) getMigrationsToRevert(executed []MigrationRecord, steps int) []MigrationRecord {
	if steps > 0 {
		if steps > len(executed) {
			steps = len(executed)
		}
		return executed[:steps]
	}

	var toRevert []MigrationRecord
	if len(executed) > 0 {
		lastBatch := executed[0].Batch
		for _, e := range executed {
			if e.Batch == lastBatch {
				toRevert = append(toRevert, e)
			} else {
				break
			}
		}
	}
	return toRevert
}

func (m *MigrationManager) executeReversions(toRevert []MigrationRecord) error {
	regMap := make(map[string]Migration)
	for _, reg := range getRegisteredMigrationsSnapshot() {
		regMap[reg.Name] = reg
	}

	for _, record := range toRevert {
		if err := m.revertSingleMigration(record, regMap); err != nil {
			return err
		}
	}

	log.Println("✅ Rollback completed successfully.")
	return nil
}

func (m *MigrationManager) revertSingleMigration(record MigrationRecord, regMap map[string]Migration) error {
	reg, exists := regMap[record.Migration]
	if !exists {
		return fmt.Errorf("rollback failed: registered migration %q not found in code", record.Migration)
	}

	if reg.Down == nil {
		return fmt.Errorf("rollback failed: migration %q does not define Down rollback logic", reg.Name)
	}

	log.Printf("  <- Reverting: %s", reg.Name)
	err := m.db.Transaction(func(tx *gorm.DB) error {
		if err := reg.Down(tx); err != nil {
			return err
		}
		return tx.Table(getCustomTableName()).Delete(&record).Error
	})

	if err != nil {
		log.Printf("  ❌ Error reverting %s: %v", reg.Name, err)
		return err
	}
	return nil
}

// Reset reverses all executed migrations sequentially under a single atomic lock.
func (m *MigrationManager) Reset() error {
	db := m.getDB()

	lockedBy, err := m.generateLockID()
	if err != nil {
		return err
	}

	if err := m.waitForLock(db, lockedBy); err != nil {
		return err
	}

	stop := m.startHeartbeat(db, lockedBy)
	defer stop()

	executed, err := m.getExecutedMigrationsDescending(db)
	if err != nil {
		return err
	}
	if len(executed) == 0 {
		return nil
	}

	return m.rollbackUnderLock(db, len(executed))
}

// Refresh resets and re-runs all migrations atomically under a single lock session.
func (m *MigrationManager) Refresh() error {
	db := m.getDB()

	lockedBy, err := m.generateLockID()
	if err != nil {
		return err
	}

	if err := m.waitForLock(db, lockedBy); err != nil {
		return err
	}

	stop := m.startHeartbeat(db, lockedBy)
	defer stop()

	executed, err := m.getExecutedMigrationsDescending(db)
	if err != nil {
		return err
	}
	if len(executed) > 0 {
		log.Println("🔄 Resetting all database migrations...")
		if err := m.rollbackUnderLock(db, len(executed)); err != nil {
			return fmt.Errorf("refresh reset failed: %w", err)
		}
	}

	log.Println("🚀 Re-running all database migrations...")
	executedMap, err := m.getExecutedMigrationsMap(db)
	if err != nil {
		return err
	}

	if err := m.validateChecksums(executedMap); err != nil {
		return err
	}

	pending := m.getPendingMigrations(executedMap)
	if len(pending) == 0 {
		return nil
	}

	sort.Slice(pending, func(i, j int) bool {
		return compareMigrationNames(pending[i].Name, pending[j].Name)
	})

	return m.executePendingMigrations(pending, m.getNextBatch(db))
}

// MigrationStatusInfo holds information about a migration's status
type MigrationStatusInfo struct {
	Name      string
	Executed  bool
	Batch     int
	AppliedAt time.Time
}

// GetStatus returns the execution status of all registered migrations.
func (m *MigrationManager) GetStatus() ([]MigrationStatusInfo, error) {
	db := m.getDB()
	executedMap, err := m.getExecutedMigrationsMap(db)
	if err != nil {
		return nil, err
	}

	var status []MigrationStatusInfo
	for _, reg := range getRegisteredMigrationsSnapshot() {
		info := MigrationStatusInfo{
			Name:     reg.Name,
			Executed: false,
		}
		if record, ok := executedMap[reg.Name]; ok {
			info.Executed = true
			info.Batch = record.Batch
			info.AppliedAt = record.CreatedAt
		}
		status = append(status, info)
	}

	return status, nil
}

// acquireLock attempts to acquire the database migration lock.
func (m *MigrationManager) acquireLock(db *gorm.DB, lockedBy string) (bool, error) {
	if err := db.AutoMigrate(&MigrationLock{}); err != nil {
		return false, err
	}

	const lockKeyCondition = "lock_key = ?"

	// Ensure the lock record exists
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
			// Ignore parallel insert conflict and fetch again
			db.Where(lockKeyCondition, "migration_lock").First(&lock)
		}
	}

	now := time.Now()
	timeout := 5 * time.Minute
	if props := config.Get[DataSourceProperties](); props != nil && props.MigrationLockTimeout > 0 {
		timeout = props.MigrationLockTimeout
	}

	// Try to acquire if unlocked
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

	// Deadlock protection: break lock if it has expired
	err = db.Where(lockKeyCondition, "migration_lock").First(&lock).Error
	if err != nil {
		return false, err
	}

	if lock.Locked && now.Sub(lock.LockedAt) > timeout {
		log.Printf("⚠️ [Migrator] Breaking stale migration lock held by %s since %v", lock.LockedBy, lock.LockedAt)
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

// refreshLock updates locked_at to keep the lock fresh during executions
func (m *MigrationManager) refreshLock(db *gorm.DB, lockedBy string) {
	err := db.Model(&MigrationLock{}).
		Where("lock_key = ? AND locked_by = ? AND locked = ?", "migration_lock", lockedBy, true).
		Update("locked_at", time.Now()).Error
	if err != nil {
		log.Printf("⚠️ [Migrator] Failed to refresh migration lock heartbeat: %v", err)
	}
}

func computeChecksum(m Migration) string {
	if m.Checksum != "" {
		return m.Checksum
	}
	hash := sha256.Sum256([]byte(m.Name))
	return hex.EncodeToString(hash[:])
}

var versionRegex = regexp.MustCompile(`^(?:[vV])?(\d+(?:[\._]\d+)*)`)

func compareMigrationNames(a, b string) bool {
	matchA := versionRegex.FindStringSubmatch(a)
	matchB := versionRegex.FindStringSubmatch(b)

	if len(matchA) > 1 && len(matchB) > 1 {
		partsA := parseNumericParts(matchA[1])
		partsB := parseNumericParts(matchB[1])

		for i := 0; i < len(partsA) && i < len(partsB); i++ {
			if partsA[i] != partsB[i] {
				return partsA[i] < partsB[i]
			}
		}
		if len(partsA) != len(partsB) {
			return len(partsA) < len(partsB)
		}
	}
	return a < b
}

func parseNumericParts(s string) []int64 {
	var parts []int64
	var current int64
	hasDigit := false
	for _, r := range s {
		if r >= '0' && r <= '9' {
			current = current*10 + int64(r-'0')
			hasDigit = true
		} else if r == '.' || r == '_' {
			if hasDigit {
				parts = append(parts, current)
				current = 0
				hasDigit = false
			}
		}
	}
	if hasDigit {
		parts = append(parts, current)
	}
	return parts
}
