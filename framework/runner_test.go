package framework

import (
	"errors"
	"testing"

	"github.com/NeftaliAcosta/springo/framework/ioc"
	"github.com/stretchr/testify/assert"
)

type mockRunner struct {
	name    string
	order   int
	runFunc func(args []string) error
}

func (m *mockRunner) Run(args []string) error {
	if m.runFunc != nil {
		return m.runFunc(args)
	}
	return nil
}

func (m *mockRunner) GetOrder() int {
	return m.order
}

func TestRunCommandLineRunners(t *testing.T) {
	container := ioc.GetContainer()
	container.Clear()

	var executionOrder []string
	testArgs := []string{"test_arg"}

	r1 := &mockRunner{
		name:  "r1",
		order: 10,
		runFunc: func(args []string) error {
			assert.Equal(t, testArgs, args)
			executionOrder = append(executionOrder, "r1")
			return nil
		},
	}
	r2 := &mockRunner{
		name:  "r2",
		order: -5,
		runFunc: func(args []string) error {
			assert.Equal(t, testArgs, args)
			executionOrder = append(executionOrder, "r2")
			return nil
		},
	}
	r3 := &mockRunner{
		name:  "r3",
		order: 0,
		runFunc: func(args []string) error {
			assert.Equal(t, testArgs, args)
			executionOrder = append(executionOrder, "r3")
			return nil
		},
	}
	r4 := &mockRunner{
		name:  "r4",
		order: 0,
		runFunc: func(args []string) error {
			assert.Equal(t, testArgs, args)
			executionOrder = append(executionOrder, "r4")
			return nil
		},
	}
	r5 := &mockRunner{
		name:  "r5",
		order: 0,
		runFunc: func(args []string) error {
			assert.Equal(t, testArgs, args)
			executionOrder = append(executionOrder, "r5")
			return errors.New("runner 5 failed") // Should log error but not crash runner execution flow
		},
	}

	// Register them in alphabetical disorder
	container.RegisterBean("runner_three", r3)
	container.RegisterBean("runner_one", r1)
	container.RegisterBean("runner_five", r5)
	container.RegisterBean("runner_two", r2)
	container.RegisterBean("runner_four", r4)

	app := &Application{}
	app.RunCommandLineRunners(testArgs)

	// Sorted Order of execution should be:
	// 1. runner_two (-5)
	// 2. runner_five (0) -> "runner_five"
	// 3. runner_four (0) -> "runner_four"
	// 4. runner_three (0) -> "runner_three"
	// 5. runner_one (10)
	expectedOrder := []string{"r2", "r5", "r4", "r3", "r1"}
	assert.Equal(t, expectedOrder, executionOrder)

	container.Clear()
}
