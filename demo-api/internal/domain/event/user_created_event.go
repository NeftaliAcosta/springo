package event

// UserCreatedEvent is fired when a new user is registered
type UserCreatedEvent struct {
	Username string
	Email    string
}
