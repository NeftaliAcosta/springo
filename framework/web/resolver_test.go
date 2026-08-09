package web

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/NeftaliAcosta/springo/framework/security"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

type customType struct {
	Value string
}

type customResolver struct{}

func (c *customResolver) SupportsParameter(paramType reflect.Type) bool {
	return paramType == reflect.TypeOf((*customType)(nil))
}

func (c *customResolver) ResolveArgument(paramType reflect.Type, r *http.Request) (any, error) {
	return &customType{Value: "resolved-value"}, nil
}

type TestDTO struct {
	Name  string `json:"name" validate:"required"`
	Email string `json:"email" validate:"required,email"`
}

func TestResolver_DefaultResolvers(t *testing.T) {
	// 1. Arrange UserInfo context
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	ctx := req.Context()
	ctx = context.WithValue(ctx, security.UserContextKey, "john_doe")
	ctx = context.WithValue(ctx, security.RolesContextKey, []string{"USER", "ADMIN"})
	ctx = context.WithValue(ctx, security.ClaimsContextKey, map[string]any{"sub": "john_doe", "email": "john@example.com"})
	req = req.WithContext(ctx)

	// 2. Resolve UserInfo
	resolver := &UserInfoResolver{}
	if !resolver.SupportsParameter(reflect.TypeOf((*security.UserInfo)(nil))) {
		t.Error("UserInfoResolver should support *security.UserInfo")
	}

	val, err := resolver.ResolveArgument(reflect.TypeOf((*security.UserInfo)(nil)), req)
	if err != nil {
		t.Fatalf("failed to resolve UserInfo: %v", err)
	}

	userInfo, ok := val.(*security.UserInfo)
	if !ok {
		t.Fatalf("expected *security.UserInfo, got %T", val)
	}

	if userInfo.Username != "john_doe" {
		t.Errorf("expected username john_doe, got %s", userInfo.Username)
	}
	if len(userInfo.Roles) != 2 || userInfo.Roles[0] != "USER" {
		t.Errorf("unexpected roles: %v", userInfo.Roles)
	}
	if userInfo.Claims["email"] != "john@example.com" {
		t.Errorf("unexpected claims: %v", userInfo.Claims)
	}

	// 3. Resolve Header
	headerResolver := &HeaderResolver{}
	if !headerResolver.SupportsParameter(reflect.TypeOf((*http.Header)(nil)).Elem()) {
		t.Error("HeaderResolver should support http.Header")
	}

	req.Header.Set("X-Custom-Test", "works")
	headerVal, err := headerResolver.ResolveArgument(reflect.TypeOf((*http.Header)(nil)).Elem(), req)
	if err != nil {
		t.Fatalf("failed to resolve headers: %v", err)
	}

	headers, ok := headerVal.(http.Header)
	if !ok {
		t.Fatalf("expected http.Header, got %T", headerVal)
	}
	if headers.Get("X-Custom-Test") != "works" {
		t.Error("header value was not resolved")
	}

	// 4. Resolve TraceID
	traceIDResolver := &TraceIDResolver{}
	if !traceIDResolver.SupportsParameter(reflect.TypeOf((*TraceID)(nil)).Elem()) {
		t.Error("TraceIDResolver should support TraceID")
	}

	// Injected trace id key
	ctx = WithTraceID(req.Context(), "uuid-trace-123")
	req = req.WithContext(ctx)

	traceVal, err := traceIDResolver.ResolveArgument(reflect.TypeOf((*TraceID)(nil)).Elem(), req)
	if err != nil {
		t.Fatalf("failed to resolve trace id: %v", err)
	}

	traceID, ok := traceVal.(TraceID)
	if !ok {
		t.Fatalf("expected TraceID, got %T", traceVal)
	}
	if traceID != "uuid-trace-123" {
		t.Errorf("expected uuid-trace-123, got %s", traceID)
	}
}

func TestDispatcher_DynamicParamResolution(t *testing.T) {
	ClearArgumentResolvers()
	RegisterArgumentResolver(&customResolver{})

	// 1. Controller with context, UserInfo, TraceID, and customType
	controllerFunc := func(ctx context.Context, user *security.UserInfo, trace TraceID, custom *customType) (any, error) {
		return map[string]any{
			"username": user.Username,
			"trace":    string(trace),
			"custom":   custom.Value,
		}, nil
	}

	handler := Dispatch(controllerFunc)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	ctx := req.Context()
	ctx = context.WithValue(ctx, security.UserContextKey, "jane_doe")
	ctx = WithTraceID(ctx, "trace-abc")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	data := resp["data"].(map[string]any)
	if data["username"] != "jane_doe" {
		t.Errorf("expected jane_doe, got %v", data["username"])
	}
	if data["trace"] != "trace-abc" {
		t.Errorf("expected trace-abc, got %v", data["trace"])
	}
	if data["custom"] != "resolved-value" {
		t.Errorf("expected resolved-value, got %v", data["custom"])
	}
}

func TestDispatcher_DTOValidationAndBinding(t *testing.T) {
	ClearArgumentResolvers()

	controllerFunc := func(ctx context.Context, req TestDTO) (any, error) {
		return map[string]string{
			"name":  req.Name,
			"email": req.Email,
		}, nil
	}

	handler := Dispatch(controllerFunc)

	// Test valid payload
	body := `{"name":"Alice","email":"alice@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", rec.Code)
	}

	// Test invalid payload (should trigger validation error)
	invalidBody := `{"name":"","email":"invalid-email"}`
	reqErr := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewBufferString(invalidBody))
	reqErr.Header.Set("Content-Type", "application/json")
	recErr := httptest.NewRecorder()

	handler.ServeHTTP(recErr, reqErr)

	if recErr.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected status 422, got %d. Body: %s", recErr.Code, recErr.Body.String())
	}
}
