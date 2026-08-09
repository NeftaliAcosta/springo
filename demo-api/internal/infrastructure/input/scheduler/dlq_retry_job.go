package scheduler

import (
	"context"
	"github.com/NeftaliAcosta/springo/framework/event"
	"github.com/NeftaliAcosta/springo/framework/scheduler"
)

func init() {
	// Register the DLQ Retry Job as a scheduled task
	scheduler.Register("DLQRetryJob", func(ctx context.Context) error {
		manager := &event.RetryManager{}
		return manager.ProcessDLQ()
	})
}
