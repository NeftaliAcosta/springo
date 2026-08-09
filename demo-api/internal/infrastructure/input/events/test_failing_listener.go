package events

import (
	"context"
	"errors"
	"github.com/NeftaliAcosta/springo/framework/event"
	"log"
)

// TestFailingEvent is used for DLQ testing
type TestFailingEvent struct {
	Message string
}

func init() {
	// Register a listener that ALWAYS fails to test DLQ persistence and retries
	event.RegisterListener(func(ctx context.Context, e TestFailingEvent) error {
		log.Printf("⚠️  [FailingListener] Intentionally failing for event: %s", e.Message)
		return errors.New("intentional enterprise system failure for DLQ test")
	})
}
