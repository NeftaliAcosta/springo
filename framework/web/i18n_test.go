package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageSource_FlattenAndGet(t *testing.T) {
	tmpDir := t.TempDir()

	esContent := `
user:
  name:
    required: "El nombre de usuario es obligatorio"
validation:
  min: "Debe tener al menos {0} caracteres"
`
	enContent := `
user:
  name:
    required: "Username is required"
validation:
  min: "Must be at least {0} characters"
generic: "Hello {0}, welcome to {1}!"
`

	err := os.WriteFile(filepath.Join(tmpDir, "messages_es.yaml"), []byte(esContent), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(tmpDir, "messages.yaml"), []byte(enContent), 0644)
	require.NoError(t, err)

	ms := NewMessageSource("en")
	err = ms.LoadTranslations(tmpDir)
	require.NoError(t, err)

	// Test es translations
	assert.Equal(t, "El nombre de usuario es obligatorio", ms.GetMessage("es", "user.name.required"))
	assert.Equal(t, "Debe tener al menos 5 caracteres", ms.GetMessage("es", "validation.min", 5))

	// Test es-MX fallback to es
	assert.Equal(t, "El nombre de usuario es obligatorio", ms.GetMessage("es-MX", "user.name.required"))

	// Test en fallback from es (key not present in es, present in en)
	assert.Equal(t, "Hello John, welcome to SprinGo!", ms.GetMessage("es", "generic", "John", "SprinGo"))

	// Test default locale directly
	assert.Equal(t, "Username is required", ms.GetMessage("en", "user.name.required"))

	// Test fallback to key itself when not found
	assert.Equal(t, "missing.key", ms.GetMessage("es", "missing.key", 123))
}

func TestI18nMiddleware(t *testing.T) {
	middleware := I18nMiddleware("en")

	tests := []struct {
		name           string
		url            string
		headers        map[string]string
		expectedLocale string
	}{
		{
			name:           "Query param priority",
			url:            "http://example.com/api?lang=fr",
			headers:        map[string]string{"Accept-Language": "es;q=0.9"},
			expectedLocale: "fr",
		},
		{
			name:           "Accept-Language header priority",
			url:            "http://example.com/api",
			headers:        map[string]string{"Accept-Language": "es-MX,es;q=0.9,en;q=0.8"},
			expectedLocale: "es-MX",
		},
		{
			name:           "Fallback locale",
			url:            "http://example.com/api",
			headers:        map[string]string{},
			expectedLocale: "en",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.url, nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			w := httptest.NewRecorder()

			called := false
			handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				locale := GetLocale(r.Context())
				assert.Equal(t, tt.expectedLocale, locale)
			}))

			handler.ServeHTTP(w, req)
			assert.True(t, called)
		})
	}
}
