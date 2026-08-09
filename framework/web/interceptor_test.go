package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

type mockInterceptor struct {
	name             string
	preHandleResult  bool
	preHandleError   error
	preHandleCalls   int
	postHandleCalls  int
	afterCompCalls   int
	afterCompError   error
	executionHistory *[]string
}

func (m *mockInterceptor) PreHandle(w http.ResponseWriter, r *http.Request) (bool, error) {
	m.preHandleCalls++
	*m.executionHistory = append(*m.executionHistory, m.name+"_pre")
	return m.preHandleResult, m.preHandleError
}

func (m *mockInterceptor) PostHandle(w http.ResponseWriter, r *http.Request) error {
	m.postHandleCalls++
	*m.executionHistory = append(*m.executionHistory, m.name+"_post")
	return nil
}

func (m *mockInterceptor) AfterCompletion(w http.ResponseWriter, r *http.Request, err error) {
	m.afterCompCalls++
	m.afterCompError = err
	*m.executionHistory = append(*m.executionHistory, m.name+"_after")
}

func TestInterceptor_MatchPath(t *testing.T) {
	tests := []struct {
		pattern  string
		path     string
		expected bool
	}{
		{"/users", "/users", true},
		{"/users", "/users/", true},
		{"/users/*", "/users/123", true},
		{"/users/*", "/users/123/profile", false},
		{"/users/**", "/users/123/profile", true},
		{"/api/v1/**", "/api/v1/users/456/posts/789", true},
		{"/users/{id}", "/users/123", true},
		{"/users/{id}/profile", "/users/123/profile", true},
		{"/users/:id", "/users/123", true},
	}

	for _, tt := range tests {
		result := MatchPath(tt.pattern, tt.path)
		if result != tt.expected {
			t.Errorf("MatchPath(%q, %q) = %v; want %v", tt.pattern, tt.path, result, tt.expected)
		}
	}
}

func TestInterceptor_RegistrationInclusionExclusion(t *testing.T) {
	ClearInterceptors()
	reg := RegisterInterceptor(&mockInterceptor{})
	reg.AddPathPatterns("/api/**").ExcludePathPatterns("/api/public/**")

	if !reg.match("/api/v1/users") {
		t.Error("expected match for /api/v1/users")
	}

	if reg.match("/api/public/health") {
		t.Error("expected no match for /api/public/health (excluded)")
	}

	if reg.match("/other") {
		t.Error("expected no match for /other")
	}
}

func TestInterceptor_FullLifecycleExecution(t *testing.T) {
	ClearInterceptors()
	history := &[]string{}

	int1 := &mockInterceptor{name: "int1", preHandleResult: true, executionHistory: history}
	int2 := &mockInterceptor{name: "int2", preHandleResult: true, executionHistory: history}

	RegisterInterceptor(int1).AddPathPatterns("/api/**")
	RegisterInterceptor(int2).AddPathPatterns("/api/**")

	handlerCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		*history = append(*history, "handler")
		w.WriteHeader(http.StatusOK)
	})

	middleware := InterceptorMiddleware(nextHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)

	if !handlerCalled {
		t.Error("handler was not called")
	}

	expectedHistory := []string{
		"int1_pre",
		"int2_pre",
		"handler",
		"int2_post",
		"int1_post",
		"int2_after",
		"int1_after",
	}

	if len(*history) != len(expectedHistory) {
		t.Fatalf("expected history length %d, got %d. Got: %v", len(expectedHistory), len(*history), *history)
	}

	for i, v := range expectedHistory {
		if (*history)[i] != v {
			t.Errorf("at index %d: expected %q, got %q", i, v, (*history)[i])
		}
	}

	if int1.afterCompError != nil || int2.afterCompError != nil {
		t.Error("expected no error in AfterCompletion")
	}
}

func TestInterceptor_AbortFlow(t *testing.T) {
	ClearInterceptors()
	history := &[]string{}

	int1 := &mockInterceptor{name: "int1", preHandleResult: true, executionHistory: history}
	int2 := &mockInterceptor{name: "int2", preHandleResult: false, executionHistory: history} // Aborts
	int3 := &mockInterceptor{name: "int3", preHandleResult: true, executionHistory: history}

	RegisterInterceptor(int1).AddPathPatterns("/api/**")
	RegisterInterceptor(int2).AddPathPatterns("/api/**")
	RegisterInterceptor(int3).AddPathPatterns("/api/**")

	handlerCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
	})

	middleware := InterceptorMiddleware(nextHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)

	if handlerCalled {
		t.Error("handler should not be called when interceptor aborts")
	}

	// Expecting:
	// - int1_pre
	// - int2_pre (aborts here)
	// - int1_after (only successfully pre-handled interceptors run afterCompletion)
	expectedHistory := []string{
		"int1_pre",
		"int2_pre",
		"int1_after",
	}

	if len(*history) != len(expectedHistory) {
		t.Fatalf("expected history length %d, got %d. Got: %v", len(expectedHistory), len(*history), *history)
	}

	for i, v := range expectedHistory {
		if (*history)[i] != v {
			t.Errorf("at index %d: expected %q, got %q", i, v, (*history)[i])
		}
	}
}

func TestInterceptor_PanicRecoveryLifecycle(t *testing.T) {
	ClearInterceptors()
	history := &[]string{}

	int1 := &mockInterceptor{name: "int1", preHandleResult: true, executionHistory: history}
	RegisterInterceptor(int1).AddPathPatterns("/api/**")

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*history = append(*history, "handler_panic")
		panic("something went wrong")
	})

	middleware := InterceptorMiddleware(nextHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	rec := httptest.NewRecorder()

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic, but it was swallowed")
		}

		// Ensure AfterCompletion was still executed on panic
		expectedHistory := []string{
			"int1_pre",
			"handler_panic",
			"int1_after",
		}

		if len(*history) != len(expectedHistory) {
			t.Fatalf("expected history length %d, got %d. Got: %v", len(expectedHistory), len(*history), *history)
		}

		for i, v := range expectedHistory {
			if (*history)[i] != v {
				t.Errorf("at index %d: expected %q, got %q", i, v, (*history)[i])
			}
		}

		if int1.afterCompError == nil || int1.afterCompError.Error() != "panic: something went wrong" {
			t.Errorf("expected panic error in AfterCompletion, got: %v", int1.afterCompError)
		}
	}()

	middleware.ServeHTTP(rec, req)
}
