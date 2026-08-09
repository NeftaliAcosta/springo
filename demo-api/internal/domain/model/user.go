package model

import (
	"time"
)

// User is a pure domain entity, free from infrastructure details
type User struct {
	ID        uint
	Username  string
	Email     string
	CreatedAt time.Time
	UpdatedAt time.Time
}
