package event

import (
	"time"
)

// FailedEventEntity represents an event that failed to be processed
type FailedEventEntity struct {
	ID           uint      `gorm:"primaryKey"`
	EventName    string    `gorm:"size:255;index"`
	Payload      string    `gorm:"type:text"`
	ListenerName string    `gorm:"size:255;index"`
	Error        string    `gorm:"type:text"`
	Retries      int       `gorm:"default:0"`
	Status       string    `gorm:"size:20;index;default:'PENDING'"` // PENDING, FAILED, COMPLETED
	TraceID      string    `gorm:"size:100;index"`
	NextRetryAt  time.Time `gorm:"index"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (FailedEventEntity) TableName() string {
	return "springo_failed_events"
}

// OutboxEventEntity represents an event stored in the database for the Outbox pattern
type OutboxEventEntity struct {
	ID        uint      `gorm:"primaryKey"`
	EventName string    `gorm:"size:255;index"`
	Payload   string    `gorm:"type:text"`
	Status    string    `gorm:"size:20;index;default:'PENDING'"` // PENDING, PROCESSED
	TraceID   string    `gorm:"size:100;index"`
	CreatedAt time.Time `gorm:"index"`
	UpdatedAt time.Time
}

// TableName returns the table name for OutboxEventEntity
func (OutboxEventEntity) TableName() string {
	return "springo_outbox"
}
