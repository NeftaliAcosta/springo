package events

import (
	"context"
	domainEvent "github.com/NeftaliAcosta/springo/demo-api/internal/domain/event"
	"github.com/NeftaliAcosta/springo/framework/event"
	"log"
)

func init() {
	// Register the listener for UserCreatedEvent
	event.RegisterListener(OnUserCreated)
}

func OnUserCreated(ctx context.Context, e domainEvent.UserCreatedEvent) error {
	log.Printf("🎉 [WelcomeListener] Sending welcome email to %s (%s)", e.Username, e.Email)
	return nil
}
