package database

import (
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/NeftaliAcosta/springo/framework/config"
	"gorm.io/gorm"
)

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
	custom := customTableName
	migrationsMu.RUnlock()
	if custom != "springo_migrations" && custom != "" {
		return custom
	}
	if props := config.Get[DataSourceProperties](); props != nil {
		return props.GetMigrationTableName()
	}
	return custom
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
	props *DataSourceProperties
	debug bool
}

// NewMigrationManager creates a new instance of MigrationManager.
func NewMigrationManager(db *gorm.DB, debug bool) *MigrationManager {
	return &MigrationManager{
		db:    db,
		props: config.Get[DataSourceProperties](),
		debug: debug,
	}
}

// NewMigrationManagerWithProps creates a MigrationManager with explicit datasource properties.
func NewMigrationManagerWithProps(db *gorm.DB, props *DataSourceProperties, debug bool) *MigrationManager {
	return &MigrationManager{
		db:    db,
		props: props,
		debug: debug,
	}
}

// SetProperties sets datasource properties for this migration manager instance.
func (m *MigrationManager) SetProperties(props *DataSourceProperties) *MigrationManager {
	m.props = props
	return m
}

func (m *MigrationManager) getProps() *DataSourceProperties {
	if m.props != nil {
		return m.props
	}
	return config.Get[DataSourceProperties]()
}

func (m *MigrationManager) getDB() *gorm.DB {
	if m.debug {
		return m.db
	}
	return m.db.Session(&gorm.Session{Logger: m.db.Logger.LogMode(1)})
}

func (m *MigrationManager) autoDiscoverMigrations() {
	props := m.getProps()
	_ = AutoDiscoverAndRegisterSQLMigrations(props)
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

func (m *MigrationManager) applyBaselineIfRequired(db *gorm.DB, executedMap *map[string]MigrationRecord) error {
	props := m.getProps()
	if props == nil || !props.Migration.BaselineOnMigrate || len(*executedMap) > 0 {
		return nil
	}

	baselineVer := props.GetBaselineVersion()
	var baselineMigrations []Migration
	for _, reg := range getRegisteredMigrationsSnapshot() {
		if isVersionLTE(reg.Name, baselineVer) {
			baselineMigrations = append(baselineMigrations, reg)
		}
	}

	if len(baselineMigrations) == 0 {
		return nil
	}

	log.Printf("ℹ️ [Migrator] Applying baseline up to version %q (%d migrations)", baselineVer, len(baselineMigrations))
	err := db.Transaction(func(tx *gorm.DB) error {
		for _, b := range baselineMigrations {
			record := MigrationRecord{
				Migration: b.Name,
				Batch:     1,
				Checksum:  computeChecksum(b),
				CreatedAt: time.Now(),
			}
			if err := tx.Table(getCustomTableName()).Create(&record).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("applying baseline migrations: %w", err)
	}

	refreshed, err := m.getExecutedMigrationsMap(db)
	if err != nil {
		return err
	}
	*executedMap = refreshed
	return nil
}

func (m *MigrationManager) validateOrder(pending []Migration, executedMap map[string]MigrationRecord) error {
	props := m.getProps()
	if (props != nil && props.Migration.OutOfOrder) || len(executedMap) == 0 || len(pending) == 0 {
		return nil
	}

	latestExecuted := findLatestExecutedMigration(executedMap)
	if latestExecuted == "" {
		return nil
	}

	for _, p := range pending {
		if compareMigrationNames(p.Name, latestExecuted) {
			return fmt.Errorf(
				"out-of-order migration detected: %q has lower version than latest applied %q (out-of-order is false)",
				p.Name, latestExecuted,
			)
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
	m.autoDiscoverMigrations()
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

	if err := m.applyBaselineIfRequired(db, &executedMap); err != nil {
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

	if err := m.validateOrder(pending, executedMap); err != nil {
		return err
	}

	if err := m.executePendingMigrations(pending, m.getNextBatch(db)); err != nil {
		return err
	}

	log.Printf("✅ Database migrations completed successfully.")
	return nil
}

// MigrationStatusInfo holds information about a migration's status.
type MigrationStatusInfo struct {
	Name      string
	Executed  bool
	Batch     int
	AppliedAt time.Time
}

// GetStatus returns the execution status of all registered migrations.
func (m *MigrationManager) GetStatus() ([]MigrationStatusInfo, error) {
	m.autoDiscoverMigrations()
	db := m.getDB()
	executedMap, err := m.getExecutedMigrationsMap(db)
	if err != nil {
		return nil, err
	}

	snapshot := getRegisteredMigrationsSnapshot()
	sort.Slice(snapshot, func(i, j int) bool {
		return compareMigrationNames(snapshot[i].Name, snapshot[j].Name)
	})

	var status []MigrationStatusInfo
	for _, reg := range snapshot {
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
