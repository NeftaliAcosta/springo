// Package lifecycle provides ordered application startup, ready, and shutdown hooks.
package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// Hook is a lifecycle callback. Returning an error from an initializer aborts bootstrap.
type Hook func(context.Context) error

type registration struct {
	name  string
	order int
	hook  Hook
}

type registry struct {
	mu    sync.RWMutex
	hooks map[string]registration
}

var (
	initializers  = newRegistry()
	readyHooks    = newRegistry()
	shutdownHooks = newRegistry()
)

func newRegistry() *registry { return &registry{hooks: make(map[string]registration)} }

// RegisterInitializer registers a fail-fast hook executed during BootstrapE after IoC initialization.
func RegisterInitializer(name string, order int, hook Hook) {
	initializers.register(name, order, hook)
}

// RegisterReady registers a hook executed after the HTTP listener is ready.
func RegisterReady(name string, order int, hook Hook) {
	readyHooks.register(name, order, hook)
}

// RegisterShutdown registers a hook executed during graceful shutdown in reverse order.
func RegisterShutdown(name string, order int, hook Hook) {
	shutdownHooks.register(name, order, hook)
}

func (r *registry) register(name string, order int, hook Hook) {
	if name == "" {
		panic("lifecycle hook name is required")
	}
	if hook == nil {
		panic("lifecycle hook cannot be nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.hooks[name]; exists {
		panic(fmt.Sprintf("lifecycle hook %q is already registered", name))
	}
	r.hooks[name] = registration{name: name, order: order, hook: hook}
}

// RunInitializers executes all initializers in ascending order and stops on the first error.
func RunInitializers(ctx context.Context) error {
	return runForward(ctx, initializers.snapshot(), true)
}

// RunReady executes ready hooks in ascending order and returns all errors.
func RunReady(ctx context.Context) error { return runForward(ctx, readyHooks.snapshot(), false) }

// RunShutdown executes shutdown hooks in descending order and returns all errors.
func RunShutdown(ctx context.Context) error {
	hooks := shutdownHooks.snapshot()
	sort.SliceStable(hooks, func(i, j int) bool {
		if hooks[i].order == hooks[j].order {
			return hooks[i].name > hooks[j].name
		}
		return hooks[i].order > hooks[j].order
	})
	return run(ctx, hooks, false)
}

func runForward(ctx context.Context, hooks []registration, failFast bool) error {
	sort.SliceStable(hooks, func(i, j int) bool {
		if hooks[i].order == hooks[j].order {
			return hooks[i].name < hooks[j].name
		}
		return hooks[i].order < hooks[j].order
	})
	return run(ctx, hooks, failFast)
}

func run(ctx context.Context, hooks []registration, failFast bool) error {
	var errs []error
	for _, item := range hooks {
		if err := invoke(ctx, item); err != nil {
			wrapped := fmt.Errorf("lifecycle hook %q failed: %w", item.name, err)
			if failFast {
				return wrapped
			}
			errs = append(errs, wrapped)
		}
	}
	return errors.Join(errs...)
}

func invoke(ctx context.Context, item registration) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("panic: %v", recovered)
		}
	}()
	return item.hook(ctx)
}

func (r *registry) snapshot() []registration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]registration, 0, len(r.hooks))
	for _, item := range r.hooks {
		result = append(result, item)
	}
	return result
}

// BackupRegistrations snapshots all hooks, clears them, and returns a restore function intended for tests.
func BackupRegistrations() func() {
	initializerSnapshot := initializers.snapshot()
	readySnapshot := readyHooks.snapshot()
	shutdownSnapshot := shutdownHooks.snapshot()
	initializers.replace(nil)
	readyHooks.replace(nil)
	shutdownHooks.replace(nil)
	return func() {
		initializers.replace(initializerSnapshot)
		readyHooks.replace(readySnapshot)
		shutdownHooks.replace(shutdownSnapshot)
	}
}

func (r *registry) replace(hooks []registration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hooks = make(map[string]registration, len(hooks))
	for _, item := range hooks {
		r.hooks[item.name] = item
	}
}
