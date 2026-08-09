package test

import (
	"strings"
)

// Additional assertions for JSON paths

// ExpectJSON asserts that the raw body matches expected JSON.
func (ra *ResponseAssert) ExpectJSON(expectedJSON string) *ResponseAssert {
	// Simple string comparison for now; enterprise version would use a JSON deep-equal library
	expectedTrimmed := strings.TrimSpace(expectedJSON)
	actualTrimmed := strings.TrimSpace(ra.body)
	if expectedTrimmed != actualTrimmed {
		ra.t.Errorf("Expected JSON:\n%s\nBut got:\n%s", expectedTrimmed, actualTrimmed)
	}
	return ra
}
