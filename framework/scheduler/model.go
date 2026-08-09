package scheduler

import "time"

// ShedLockEntity represents a distributed lock record for scheduled tasks
type ShedLockEntity struct {
	Name      string    `gorm:"primaryKey;size:255"`
	LockUntil time.Time `gorm:"index"`
	LockedAt  time.Time
	LockedBy  string `gorm:"size:255"`
}

// TableName returns the custom table name for ShedLockEntity
func (ShedLockEntity) TableName() string {
	return "springo_shedlock"
}
