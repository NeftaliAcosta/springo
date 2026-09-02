package lifecycle

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func isolateRegistrations(t *testing.T) {
	t.Helper()
	restore := BackupRegistrations()
	t.Cleanup(restore)
}

func TestLifecycleOrder(t *testing.T) {
	isolateRegistrations(t)

	var calls []string
	appendCall := func(name string) Hook {
		return func(context.Context) error {
			calls = append(calls, name)
			return nil
		}
	}

	RegisterInitializer("second", 20, appendCall("initializer-second"))
	RegisterInitializer("first", 10, appendCall("initializer-first"))
	RegisterReady("second", 20, appendCall("ready-second"))
	RegisterReady("first", 10, appendCall("ready-first"))
	RegisterShutdown("second", 20, appendCall("shutdown-second"))
	RegisterShutdown("first", 10, appendCall("shutdown-first"))

	if err := RunInitializers(context.Background()); err != nil {
		t.Fatalf("RunInitializers() error = %v", err)
	}
	if err := RunReady(context.Background()); err != nil {
		t.Fatalf("RunReady() error = %v", err)
	}
	if err := RunShutdown(context.Background()); err != nil {
		t.Fatalf("RunShutdown() error = %v", err)
	}

	want := []string{
		"initializer-first", "initializer-second",
		"ready-first", "ready-second",
		"shutdown-second", "shutdown-first",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestInitializersFailFast(t *testing.T) {
	isolateRegistrations(t)

	var calls []string
	RegisterInitializer("failing", 10, func(context.Context) error {
		calls = append(calls, "failing")
		return errors.New("boom")
	})
	RegisterInitializer("not-called", 20, func(context.Context) error {
		calls = append(calls, "not-called")
		return nil
	})

	err := RunInitializers(context.Background())
	if err == nil || !strings.Contains(err.Error(), `lifecycle hook "failing" failed: boom`) {
		t.Fatalf("RunInitializers() error = %v", err)
	}
	if !reflect.DeepEqual(calls, []string{"failing"}) {
		t.Fatalf("calls = %v", calls)
	}
}

func TestReadyAndShutdownAggregateErrors(t *testing.T) {
	isolateRegistrations(t)

	RegisterReady("one", 10, func(context.Context) error { return errors.New("ready one") })
	RegisterReady("two", 20, func(context.Context) error { return errors.New("ready two") })
	RegisterShutdown("one", 10, func(context.Context) error { return errors.New("shutdown one") })
	RegisterShutdown("two", 20, func(context.Context) error { return errors.New("shutdown two") })

	for name, err := range map[string]error{
		"ready":    RunReady(context.Background()),
		"shutdown": RunShutdown(context.Background()),
	} {
		if err == nil || !strings.Contains(err.Error(), "one") || !strings.Contains(err.Error(), "two") {
			t.Fatalf("%s error = %v", name, err)
		}
	}
}

func TestHookPanicBecomesError(t *testing.T) {
	isolateRegistrations(t)

	RegisterInitializer("panic", 0, func(context.Context) error { panic("broken") })
	err := RunInitializers(context.Background())
	if err == nil || !strings.Contains(err.Error(), "panic: broken") {
		t.Fatalf("RunInitializers() error = %v", err)
	}
}

func TestRegistrationValidation(t *testing.T) {
	tests := map[string]func(){
		"empty name": func() { RegisterInitializer("", 0, func(context.Context) error { return nil }) },
		"nil hook":   func() { RegisterReady("nil", 0, nil) },
		"duplicate": func() {
			RegisterShutdown("duplicate", 0, func(context.Context) error { return nil })
			RegisterShutdown("duplicate", 0, func(context.Context) error { return nil })
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			isolateRegistrations(t)
			defer func() {
				if recover() == nil {
					t.Fatal("expected panic")
				}
			}()
			test()
		})
	}
}

func TestBackupRegistrationsClearsAndRestores(t *testing.T) {
	isolateRegistrations(t)

	called := 0
	RegisterInitializer("original", 0, func(context.Context) error {
		called++
		return nil
	})
	restore := BackupRegistrations()

	if err := RunInitializers(context.Background()); err != nil {
		t.Fatalf("RunInitializers() error = %v", err)
	}
	if called != 0 {
		t.Fatalf("called = %d before restore", called)
	}

	restore()
	if err := RunInitializers(context.Background()); err != nil {
		t.Fatalf("RunInitializers() after restore error = %v", err)
	}
	if called != 1 {
		t.Fatalf("called = %d after restore", called)
	}
}
