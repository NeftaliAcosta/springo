package database

import (
	"fmt"
	"log"
	"sort"

	"gorm.io/gorm"
)

// Rollback reverses the last 'steps' migrations (or the entire last batch if steps <= 0).
func (m *MigrationManager) Rollback(steps int) error {
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
