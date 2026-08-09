package scheduler

import (
	"context"
	"github.com/NeftaliAcosta/springo/framework/scheduler"
	"log"
)

func init() {
	// Register the job in the framework's engine
	scheduler.Register("AuthTokenJob", func(ctx context.Context) error {
		log.Println("[Job: AuthTokenJob] Refreshing external API token...")
		// Simulate some work
		// token, err := externalAuthService.GetToken()
		log.Println("[Job: AuthTokenJob] ✅ Token refreshed successfully")
		return nil
	})
}
