package security

import (
	"os"
	"testing"
)

// TestJwtProperties_Validate verifies that JwtProperties enforces robust security
// policies when executing under the "prod" profile, while remaining permissive in development.
func TestJwtProperties_Validate(t *testing.T) {
	tests := []struct {
		name        string
		profile     string
		secret      string
		expectError bool
	}{
		{
			name:        "Dev profile with weak secret is acceptable",
			profile:     "dev",
			secret:      "weak",
			expectError: false,
		},
		{
			name:        "Prod profile with secure secret is acceptable",
			profile:     "prod",
			secret:      "a-very-secure-secret-of-32-chars-long",
			expectError: false,
		},
		{
			name:        "Prod profile with empty secret is rejected",
			profile:     "prod",
			secret:      "",
			expectError: true,
		},
		{
			name:        "Prod profile with default dev secret is rejected",
			profile:     "prod",
			secret:      "springo-ultra-secret-key-for-development",
			expectError: true,
		},
		{
			name:        "Prod profile with weak secret is rejected",
			profile:     "prod",
			secret:      "too-short-secret",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			oldProfile := os.Getenv("SPRINGO_PROFILES_ACTIVE")
			defer os.Setenv("SPRINGO_PROFILES_ACTIVE", oldProfile)
			os.Setenv("SPRINGO_PROFILES_ACTIVE", tt.profile)

			props := &JwtProperties{
				Secret:     tt.secret,
				Expiration: 60,
			}

			// Act
			err := props.Validate()

			// Assert
			if (err != nil) != tt.expectError {
				t.Errorf("Validate() error = %v, expectError = %v", err, tt.expectError)
			}
		})
	}
}
