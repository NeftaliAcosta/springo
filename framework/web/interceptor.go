package web

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
)

// HandlerInterceptor defines the contract for Spring-like request interception.
type HandlerInterceptor interface {
	// PreHandle is executed before the handler is invoked.
	// If it returns false, request processing is halted and AfterCompletion is run.
	PreHandle(w http.ResponseWriter, r *http.Request) (bool, error)
	// PostHandle is executed after the handler is invoked but before response is committed (if not already written).
	PostHandle(w http.ResponseWriter, r *http.Request) error
	// AfterCompletion is executed after the request has finished (even in case of panic/errors).
	AfterCompletion(w http.ResponseWriter, r *http.Request, err error)
}

// InterceptorRegistration configures routing mapping for an interceptor.
type InterceptorRegistration struct {
	interceptor HandlerInterceptor
	includes    []string
	excludes    []string
}

// AddPathPatterns registers URL patterns to match.
func (ir *InterceptorRegistration) AddPathPatterns(patterns ...string) *InterceptorRegistration {
	ir.includes = append(ir.includes, patterns...)
	return ir
}

// ExcludePathPatterns registers URL patterns to exclude.
func (ir *InterceptorRegistration) ExcludePathPatterns(patterns ...string) *InterceptorRegistration {
	ir.excludes = append(ir.excludes, patterns...)
	return ir
}

// match checks if the given path matches the registration inclusions and exclusions.
func (ir *InterceptorRegistration) match(path string) bool {
	// Check exclusions first
	for _, pattern := range ir.excludes {
		if MatchPath(pattern, path) {
			return false
		}
	}

	// If no includes are specified, we default to match all
	if len(ir.includes) == 0 {
		return true
	}

	// Check inclusions
	for _, pattern := range ir.includes {
		if MatchPath(pattern, path) {
			return true
		}
	}

	return false
}

// interceptorRegistry holds all registered interceptors.
type interceptorRegistry struct {
	mu            sync.RWMutex
	registrations []*InterceptorRegistration
}

var interceptorRegistryInstance = &interceptorRegistry{}

// RegisterInterceptor registers a new handler interceptor in the framework.
func RegisterInterceptor(interceptor HandlerInterceptor) *InterceptorRegistration {
	interceptorRegistryInstance.mu.Lock()
	defer interceptorRegistryInstance.mu.Unlock()

	registration := &InterceptorRegistration{
		interceptor: interceptor,
	}
	interceptorRegistryInstance.registrations = append(interceptorRegistryInstance.registrations, registration)
	return registration
}

// getMatchingInterceptors returns all registrations that match the given request path.
func getMatchingInterceptors(path string) []*InterceptorRegistration {
	interceptorRegistryInstance.mu.RLock()
	defer interceptorRegistryInstance.mu.RUnlock()

	var matched []*InterceptorRegistration
	for _, reg := range interceptorRegistryInstance.registrations {
		if reg.match(path) {
			matched = append(matched, reg)
		}
	}
	return matched
}

// ClearInterceptors clears all registered interceptors (mainly for testing).
func ClearInterceptors() {
	interceptorRegistryInstance.mu.Lock()
	defer interceptorRegistryInstance.mu.Unlock()
	interceptorRegistryInstance.registrations = nil
}

// MatchPath checks if a path matches a Spring-like pattern.
func MatchPath(pattern, path string) bool {
	pattern = strings.Trim(pattern, "/")
	path = strings.Trim(path, "/")

	if pattern == "" && path == "" {
		return true
	}
	if pattern == "" || path == "" {
		return pattern == "**"
	}

	patternParts := strings.Split(pattern, "/")
	pathParts := strings.Split(path, "/")

	return matchParts(patternParts, pathParts)
}

func matchParts(patternParts, pathParts []string) bool {
	if len(patternParts) == 0 && len(pathParts) == 0 {
		return true
	}

	if len(patternParts) == 0 {
		return false
	}

	pHead := patternParts[0]

	if pHead == "**" {
		return matchDoubleStar(patternParts, pathParts)
	}

	if len(pathParts) == 0 {
		return false
	}

	if !matchSingleSegment(pHead, pathParts[0]) {
		return false
	}

	return matchParts(patternParts[1:], pathParts[1:])
}

func matchDoubleStar(patternParts, pathParts []string) bool {
	if len(patternParts) == 1 {
		return true
	}
	for i := 0; i <= len(pathParts); i++ {
		if matchParts(patternParts[1:], pathParts[i:]) {
			return true
		}
	}
	return false
}

func matchSingleSegment(patternPart, pathPart string) bool {
	if patternPart == "*" {
		return true
	}
	if strings.HasPrefix(patternPart, "{") && strings.HasSuffix(patternPart, "}") {
		return true
	}
	if strings.HasPrefix(patternPart, ":") {
		return true
	}
	return patternPart == pathPart
}

// InterceptorMiddleware handles running the interceptor lifecycle for matching interceptors.
func InterceptorMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		matched := getMatchingInterceptors(r.URL.Path)
		if len(matched) == 0 {
			next.ServeHTTP(w, r)
			return
		}

		lastExecutedIndex, err, proceed := runPreHandle(matched, w, r)
		if err != nil || !proceed {
			if err != nil {
				HandleError(w, r, err)
			}
			return
		}

		defer runDeferredAfterCompletion(matched, &lastExecutedIndex, w, r)

		next.ServeHTTP(w, r)

		runPostHandle(matched, lastExecutedIndex, w, r)
	})
}

func runPreHandle(matched []*InterceptorRegistration, w http.ResponseWriter, r *http.Request) (int, error, bool) {
	lastExecutedIndex := -1
	for i, reg := range matched {
		proceed, err := reg.interceptor.PreHandle(w, r)
		if err != nil || !proceed {
			triggerAfterCompletion(matched, lastExecutedIndex, w, r, err)
			return lastExecutedIndex, err, false
		}
		lastExecutedIndex = i
	}
	return lastExecutedIndex, nil, true
}

func runPostHandle(matched []*InterceptorRegistration, lastExecutedIndex int, w http.ResponseWriter, r *http.Request) {
	for i := lastExecutedIndex; i >= 0; i-- {
		if err := matched[i].interceptor.PostHandle(w, r); err != nil {
			log.Printf("web: error executing PostHandle for interceptor: %v", err)
		}
	}
	triggerAfterCompletion(matched, lastExecutedIndex, w, r, nil)
}

func runDeferredAfterCompletion(matched []*InterceptorRegistration, lastExecutedIndex *int, w http.ResponseWriter, r *http.Request) {
	if rec := recover(); rec != nil {
		panicErr := fmt.Errorf("panic: %v", rec)
		triggerAfterCompletion(matched, *lastExecutedIndex, w, r, panicErr)
		panic(rec)
	}
}

func triggerAfterCompletion(matched []*InterceptorRegistration, maxIndex int, w http.ResponseWriter, r *http.Request, err error) {
	for i := maxIndex; i >= 0; i-- {
		matched[i].interceptor.AfterCompletion(w, r, err)
	}
}
