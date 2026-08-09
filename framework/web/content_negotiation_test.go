package web

import (
	"context"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

type ContentNegDTO struct {
	Name  string `json:"name" yaml:"name" xml:"name" validate:"required"`
	Email string `json:"email" yaml:"email" xml:"email" validate:"required,email"`
}

func TestResolveNegotiatedFormat(t *testing.T) {
	tests := []struct {
		name     string
		accept   string
		formatQP string
		expected string
	}{
		{
			name:     "No Accept header defaults to JSON",
			accept:   "",
			expected: "json",
		},
		{
			name:     "Wildcard accept defaults to JSON",
			accept:   "*/*",
			expected: "json",
		},
		{
			name:     "Explicit application/json",
			accept:   "application/json",
			expected: "json",
		},
		{
			name:     "Explicit application/xml",
			accept:   "application/xml",
			expected: "xml",
		},
		{
			name:     "Explicit text/xml",
			accept:   "text/xml",
			expected: "xml",
		},
		{
			name:     "Explicit application/x-yaml",
			accept:   "application/x-yaml",
			expected: "yaml",
		},
		{
			name:     "Explicit text/yaml",
			accept:   "text/yaml",
			expected: "yaml",
		},
		{
			name:     "Q-factor prioritizing JSON over XML",
			accept:   "application/xml;q=0.8,application/json;q=0.9",
			expected: "json",
		},
		{
			name:     "Q-factor prioritizing XML over JSON",
			accept:   "application/xml;q=0.95,application/json;q=0.5",
			expected: "xml",
		},
		{
			name:     "Q-factor prioritizing YAML over XML",
			accept:   "application/x-yaml;q=0.9,application/xml;q=0.7",
			expected: "yaml",
		},
		{
			name:     "Query param format=xml overrides Accept=json",
			accept:   "application/json",
			formatQP: "xml",
			expected: "xml",
		},
		{
			name:     "Query param format=yaml overrides Accept=xml",
			accept:   "application/xml",
			formatQP: "yaml",
			expected: "yaml",
		},
		{
			name:     "Invalid query param fallback to Accept header",
			accept:   "application/xml",
			formatQP: "invalid-format",
			expected: "xml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/test"
			if tt.formatQP != "" {
				url += "?format=" + tt.formatQP
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			if tt.accept != "" {
				req.Header.Set("Accept", tt.accept)
			}

			format := ResolveNegotiatedFormat(req)
			assert.Equal(t, tt.expected, format)
		})
	}
}

func TestWriteResponse_JSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()

	data := NewSuccessResponse(http.StatusOK, map[string]string{"message": "hello"})
	WriteResponse(w, req, http.StatusOK, data)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), `"status":200`)
	assert.Contains(t, w.Body.String(), `"message":"hello"`)
}

func TestWriteResponse_XML(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Accept", "application/xml")
	w := httptest.NewRecorder()

	data := NewSuccessResponse(http.StatusOK, "working")
	WriteResponse(w, req, http.StatusOK, data)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/xml; charset=utf-8", w.Header().Get("Content-Type"))
	assert.True(t, strings.HasPrefix(w.Body.String(), xml.Header))
	assert.Contains(t, w.Body.String(), `<response>`)
	assert.Contains(t, w.Body.String(), `<status>200</status>`)
	assert.Contains(t, w.Body.String(), `<data>working</data>`)
}

func TestWriteResponse_YAML(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Accept", "application/x-yaml")
	w := httptest.NewRecorder()

	data := NewSuccessResponse(http.StatusOK, map[string]string{"key": "value"})
	WriteResponse(w, req, http.StatusOK, data)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/x-yaml; charset=utf-8", w.Header().Get("Content-Type"))

	var decoded ApiResponse[map[string]string]
	err := yaml.Unmarshal(w.Body.Bytes(), &decoded)
	assert.NoError(t, err)
	assert.Equal(t, 200, decoded.Status)
	assert.Equal(t, "value", decoded.Data["key"])
}

func TestWriteResponse_XML_MapSerialization(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Accept", "application/xml")
	w := httptest.NewRecorder()

	// Dynamic map that standard xml.Marshal normally fails to encode
	dynamicMap := map[string]any{
		"user name": "John Doe", // Needs sanitization: space to underscore
		"age":       30,
		"roles":     []any{"admin", "user"},
		"address": map[string]any{
			"city": "Madrid",
		},
	}

	data := NewSuccessResponse(http.StatusOK, dynamicMap)
	WriteResponse(w, req, http.StatusOK, data)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/xml; charset=utf-8", w.Header().Get("Content-Type"))

	bodyStr := w.Body.String()
	assert.Contains(t, bodyStr, `<user_name>John Doe</user_name>`) // sanitized space
	assert.Contains(t, bodyStr, `<age>30</age>`)
	assert.Contains(t, bodyStr, `<city>Madrid</city>`)
}

func TestDispatcher_ContentNegotiation(t *testing.T) {
	// A simple controller function to dispatch
	handlerFn := func(ctx context.Context, dto *ContentNegDTO) (*ContentNegDTO, error) {
		return dto, nil
	}

	dispatcher := Dispatch(handlerFn)

	t.Run("XML Success Response", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"name":"Alex", "email":"alex@test.com"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/xml")
		w := httptest.NewRecorder()

		dispatcher.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		assert.Equal(t, "application/xml; charset=utf-8", w.Header().Get("Content-Type"))
		assert.Contains(t, w.Body.String(), `<response>`)
		assert.Contains(t, w.Body.String(), `<name>Alex</name>`)
		assert.Contains(t, w.Body.String(), `<email>alex@test.com</email>`)
	})

	t.Run("YAML ValidationError Response", func(t *testing.T) {
		// Missing fields triggers validation error (422)
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/x-yaml")
		w := httptest.NewRecorder()

		dispatcher.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
		assert.Equal(t, "application/x-yaml; charset=utf-8", w.Header().Get("Content-Type"))

		var errRes ApiResponse[any]
		err := yaml.Unmarshal(w.Body.Bytes(), &errRes)
		assert.NoError(t, err)
		assert.Equal(t, 422, errRes.Status)
		assert.Equal(t, "VALIDATION_ERROR", errRes.Code)

		errorsMap, ok := errRes.Errors.(map[string]any)
		assert.True(t, ok)
		assert.NotEmpty(t, errorsMap["name"])
		assert.NotEmpty(t, errorsMap["email"])
	})

	t.Run("XML ValidationError Response with Map Handling", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"email":"bad-email"}`)) // missing name, bad email
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/xml")
		w := httptest.NewRecorder()

		dispatcher.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
		assert.Equal(t, "application/xml; charset=utf-8", w.Header().Get("Content-Type"))

		bodyStr := w.Body.String()
		assert.Contains(t, bodyStr, `<response>`)
		assert.Contains(t, bodyStr, `<status>422</status>`)
		assert.Contains(t, bodyStr, `<code>VALIDATION_ERROR</code>`)
		// Error keys inside map translated to XML tags
		assert.Contains(t, bodyStr, `<errors>`)
		assert.Contains(t, bodyStr, `<name>`)
		assert.Contains(t, bodyStr, `<email>`)
	})
}
