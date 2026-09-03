package security

import (
	"testing"
)

// TestJwtProperties_Validate verifies that JwtProperties enforces robust security
// policies when executing under production and staging profiles, while remaining permissive in development.
func TestJwtProperties_Validate(t *testing.T) {
	tests := []struct {
		name        string
		profile     string
		secret      string
		algorithm   string
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
			name:        "Production profile alias with secure secret is acceptable",
			profile:     "production",
			secret:      "a-very-secure-secret-of-32-chars-long",
			expectError: false,
		},
		{
			name:        "Staging profile with weak secret is rejected",
			profile:     "staging",
			secret:      "weak",
			expectError: true,
		},
		{
			name:        "Stage profile alias with weak secret is rejected",
			profile:     "stage",
			secret:      "weak",
			expectError: true,
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
		{
			name:        "Unsupported algorithm none is rejected in dev",
			profile:     "dev",
			algorithm:   "none",
			expectError: true,
		},
		{
			name:        "Unsupported algorithm ES256 is rejected",
			profile:     "prod",
			algorithm:   "ES256",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			t.Setenv("SPRINGO_PROFILES_ACTIVE", tt.profile)

			props := &JwtProperties{
				Secret:     tt.secret,
				Expiration: 60,
				Algorithm:  tt.algorithm,
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
