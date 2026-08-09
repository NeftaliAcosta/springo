package scheduler

import (
	"context"
	"github.com/NeftaliAcosta/springo/framework/scheduler"
	"log"
)

func init() {
	// Register the job in the framework's engine
	scheduler.Register("CleanupOldSessionsJob", func(ctx context.Context) error {
		log.Println("[Job: CleanupOldSessionsJob] Cleaning up expired sessions...")
		// Simulate some work
		log.Println("[Job: CleanupOldSessionsJob] ✅ Cleanup completed")
		return nil
	})
}
