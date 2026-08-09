package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestClient provides a fluent API for executing HTTP requests against the application router
type TestClient struct {
	router http.Handler
	t      *testing.T
}

// ResponseAssert provides fluent assertions for the HTTP response
type ResponseAssert struct {
	t    *testing.T
	res  *httptest.ResponseRecorder
	body string
}

// NewTestClient creates a new fluent test client
func NewTestClient(t *testing.T, router http.Handler) *TestClient {
	return &TestClient{router: router, t: t}
}

// RequestBuilder helps construct an HTTP request fluently
type RequestBuilder struct {
	c       *TestClient
	method  string
	path    string
	headers map[string]string
	body    io.Reader
}

func (c *TestClient) Get(path string) *RequestBuilder {
	return &RequestBuilder{c: c, method: http.MethodGet, path: path, headers: make(map[string]string)}
}

func (c *TestClient) Post(path string) *RequestBuilder {
	return &RequestBuilder{c: c, method: http.MethodPost, path: path, headers: make(map[string]string)}
}

func (c *TestClient) Put(path string) *RequestBuilder {
	return &RequestBuilder{c: c, method: http.MethodPut, path: path, headers: make(map[string]string)}
}

func (c *TestClient) Delete(path string) *RequestBuilder {
	return &RequestBuilder{c: c, method: http.MethodDelete, path: path, headers: make(map[string]string)}
}

func (rb *RequestBuilder) WithHeader(key, value string) *RequestBuilder {
	rb.headers[key] = value
	return rb
}

func (rb *RequestBuilder) WithJSON(payload interface{}) *RequestBuilder {
	b, err := json.Marshal(payload)
	if err != nil {
		rb.c.t.Fatalf("Failed to marshal JSON payload: %v", err)
	}
	rb.body = bytes.NewBuffer(b)
	rb.headers["Content-Type"] = "application/json"
	return rb
}

// Execute performs the HTTP request and returns an asserter
func (rb *RequestBuilder) Execute() *ResponseAssert {
	req := httptest.NewRequest(rb.method, rb.path, rb.body)
	for k, v := range rb.headers {
		req.Header.Set(k, v)
	}

	w := httptest.NewRecorder()
	rb.c.router.ServeHTTP(w, req)

	return &ResponseAssert{
		t:    rb.c.t,
		res:  w,
		body: w.Body.String(),
	}
}

// ExpectStatus asserts the HTTP response status code
func (ra *ResponseAssert) ExpectStatus(expected int) *ResponseAssert {
	if ra.res.Code != expected {
		ra.t.Errorf("Expected status %d but got %d. Body: %s", expected, ra.res.Code, ra.body)
	}
	return ra
}

// ExpectBodyContains asserts that the response body contains a specific string
func (ra *ResponseAssert) ExpectBodyContains(expected string) *ResponseAssert {
	if !strings.Contains(ra.body, expected) {
		ra.t.Errorf("Expected body to contain '%s', but got: %s", expected, ra.body)
	}
	return ra
}

// PrintBody logs the response body for debugging
func (ra *ResponseAssert) PrintBody() *ResponseAssert {
	fmt.Printf("Response Body: %s\n", ra.body)
	return ra
}
