package event

import (
	"context"
	"log/slog"
	"time"

	"github.com/NeftaliAcosta/springo/framework/config"
	"github.com/NeftaliAcosta/springo/framework/ioc"
	"github.com/NeftaliAcosta/springo/framework/logging"
	"github.com/NeftaliAcosta/springo/framework/web"

	"gorm.io/gorm"
)

// RetryManager scans the DLQ and manages the retry state of failed events
type RetryManager struct{}

// ProcessDLQ is the task that handles scanning and state transition for retries
func (m *RetryManager) ProcessDLQ() error {
	props := config.Get[EventProperties]()
	if props == nil {
		if bean := ioc.GetContainer().GetBean("EventProperties"); bean != nil {
			props = bean.(*EventProperties)
		}
	}
	if props == nil || !props.DLQ.Enabled {
		return nil
	}

	db := ioc.GetContainer().GetDB()
	if db == nil {
		return nil
	}
	// Recover leases left by a crashed worker before selecting new work.
	staleBefore := time.Now().Add(-5 * time.Minute)
	if err := db.Model(&FailedEventEntity{}).
		Where("status = ? AND updated_at < ?", "RETRYING", staleBefore).
		Updates(map[string]interface{}{"status": "FAILED"}).Error; err != nil {
		return err
	}

	var failedEvents []FailedEventEntity
	// Find events that are eligible for retry (status is PENDING or FAILED and time has come or retries == 0)
	now := time.Now()
	err := db.Where("status IN ? AND retries < ? AND (next_retry_at <= ? OR retries = 0 OR next_retry_at IS NULL)",
		[]string{"PENDING", "FAILED"},
		props.DLQ.MaxRetries,
		now).
		Limit(10).
		Find(&failedEvents).Error

	if err != nil {
		return err
	}

	if len(failedEvents) == 0 {
		return nil
	}

	publisher := GetPublisher().(*defaultEventPublisher)
	for _, fe := range failedEvents {
		m.updateRetryState(db, publisher, fe, props)
	}

	return nil
}

func (m *RetryManager) updateRetryState(
	db *gorm.DB,
	p *defaultEventPublisher,
	fe FailedEventEntity,
	props *EventProperties,
) {
	nextRetry := CalculateNextRetry(fe.Retries+1, props)
	res := db.Model(&FailedEventEntity{}).
		Where("id = ? AND status IN ?", fe.ID, []string{"PENDING", "FAILED"}).
		Updates(map[string]interface{}{
			"status":        "RETRYING",
			"retries":       gorm.Expr("retries + 1"),
			"next_retry_at": nextRetry,
		})
	if res.Error != nil || res.RowsAffected == 0 {
		return
	}

	fe.Retries++
	fe.Status = "RETRYING"
	fe.NextRetryAt = nextRetry

	slog.Info("Attempting recovery for event",
		slog.String(logging.SubsystemKey, logging.FrameworkSubsystem),
		slog.String("event_name", fe.EventName),
		slog.Uint64("event_id", uint64(fe.ID)),
		slog.Int("attempt", fe.Retries),
		slog.Int("max_retries", props.DLQ.MaxRetries),
		slog.String("next_retry", fe.NextRetryAt.Format("15:04:05")),
	)

	ctx := context.Background()
	if fe.TraceID != "" {
		ctx = web.WithTraceID(ctx, fe.TraceID)
	}

	if err := RedispatchEventForListener(ctx, fe.EventName, fe.ListenerName, fe.Payload); err != nil {
		slog.Warn("Recovery attempt failed for event",
			slog.String(logging.SubsystemKey, logging.FrameworkSubsystem),
			slog.Uint64("event_id", uint64(fe.ID)),
			slog.Any("error", err),
		)
		if fe.Retries >= props.DLQ.MaxRetries {
			fe.Status = "FAILED"
			fe.Error = err.Error()
			slog.Error("Event reached MAX retries and is definitively FAILED",
				slog.String(logging.SubsystemKey, logging.FrameworkSubsystem),
				slog.String("event_name", fe.EventName),
				slog.Uint64("event_id", uint64(fe.ID)),
			)
		} else {
			fe.Status = "FAILED"
			fe.Error = err.Error()
		}
		_ = db.Save(&fe).Error
	} else {
		slog.Info("Event successfully recovered and removed from DLQ",
			slog.String(logging.SubsystemKey, logging.FrameworkSubsystem),
			slog.String("event_name", fe.EventName),
			slog.Uint64("event_id", uint64(fe.ID)),
		)
		_ = db.Delete(&FailedEventEntity{}, fe.ID).Error
	}
}
