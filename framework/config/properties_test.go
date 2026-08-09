package config

import (
	"testing"
)

type NestedConfig struct {
	Value string
}

type ComplexProperties struct {
	Slice  []string
	Map    map[string]int
	Ptr    *NestedConfig
	Struct NestedConfig
}

func TestResetProperties_DeepClone(t *testing.T) {
	// 1. Prepare target properties with initial default values
	initial := &ComplexProperties{
		Slice: []string{"initial-1", "initial-2"},
		Map: map[string]int{
			"key-1": 1,
			"key-2": 2,
		},
		Ptr: &NestedConfig{
			Value: "ptr-initial",
		},
		Struct: NestedConfig{
			Value: "struct-initial",
		},
	}

	// Register it
	RegisterProperties("test.complex", initial)
	t.Cleanup(func() {
		configMu.Lock()
		defer configMu.Unlock()
		for i := len(registry) - 1; i >= 0; i-- {
			if registry[i].target == initial {
				registry = append(registry[:i], registry[i+1:]...)
				break
			}
		}
		delete(defaultsMap, initial)
	})

	// 2. Initialize loader (dummy loader) and call InitializeProperties to capture defaultsMap
	loader := NewConfigLoader()
	// Set some property to bind or let it run without any bind matches so it keeps initial defaults
	if err := InitializeProperties(loader); err != nil {
		t.Fatalf("InitializeProperties failed: %v", err)
	}

	// 3. Mutate values in-place (shallow and deep modifications)
	initial.Slice[0] = "mutated-1"
	initial.Slice = append(initial.Slice, "mutated-3")
	initial.Map["key-1"] = 999
	initial.Map["new-key"] = 1000
	initial.Ptr.Value = "ptr-mutated"
	initial.Struct.Value = "struct-mutated"

	// 4. Call ResetProperties to restore original captured values
	ResetProperties()

	// 5. Assert that the values were completely rolled back and NOT affected by deep mutations
	if len(initial.Slice) != 2 || initial.Slice[0] != "initial-1" {
		t.Errorf("Slice was not restored correctly: %v", initial.Slice)
	}

	if initial.Map["key-1"] != 1 || len(initial.Map) != 2 {
		t.Errorf("Map was not restored correctly: %v", initial.Map)
	}

	if initial.Ptr.Value != "ptr-initial" {
		t.Errorf("Pointer struct value was not restored correctly: %s", initial.Ptr.Value)
	}

	if initial.Struct.Value != "struct-initial" {
		t.Errorf("Nested struct value was not restored correctly: %s", initial.Struct.Value)
	}
}
