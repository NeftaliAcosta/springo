package config

import (
	"os"
	"strings"
	"sync"

	"github.com/NeftaliAcosta/springo/framework/ioc"
)

type propertyAction struct {
	property string
	expected any
	fn       func()
}

type profileAction struct {
	profiles []string
	fn       func()
}

type conditionalBean struct {
	name     string
	property string
	expected any
	factory  any
}

var (
	conditionalMu        sync.RWMutex
	propertyConditionals []propertyAction
	profileConditionals  []profileAction
	conditionalBeans     []conditionalBean
)

// When registers a callback executed during bootstrap if the property matches the expected value.
func When(property string, expected any, fn func()) {
	if fn == nil || strings.TrimSpace(property) == "" {
		return
	}
	conditionalMu.Lock()
	defer conditionalMu.Unlock()
	propertyConditionals = append(propertyConditionals, propertyAction{
		property: property,
		expected: expected,
		fn:       fn,
	})
}

// WhenProfile registers a callback executed during bootstrap if active profile matches.
// The profiles parameter can be a single string (e.g. "prod") or a slice of strings ([]string{"dev", "local"}).
func WhenProfile(profiles any, fn func()) {
	if fn == nil || profiles == nil {
		return
	}

	profList := extractProfileList(profiles)
	if len(profList) == 0 {
		return
	}

	conditionalMu.Lock()
	defer conditionalMu.Unlock()
	profileConditionals = append(profileConditionals, profileAction{
		profiles: profList,
		fn:       fn,
	})
}

func extractProfileList(profiles any) []string {
	switch p := profiles.(type) {
	case string:
		clean := strings.TrimSpace(p)
		if clean != "" {
			return []string{clean}
		}
		return nil
	case []string:
		return p
	default:
		return nil
	}
}

// RegisterConditionalBean registers a bean in IoC container guarded by a property check.
func RegisterConditionalBean(name, property string, expected, factory any) {
	if factory == nil || strings.TrimSpace(name) == "" || strings.TrimSpace(property) == "" {
		return
	}
	conditionalMu.Lock()
	defer conditionalMu.Unlock()
	conditionalBeans = append(conditionalBeans, conditionalBean{
		name:     name,
		property: property,
		expected: expected,
		factory:  factory,
	})
}

// ClearConditionals clears registered conditional actions (useful for test isolation).
func ClearConditionals() {
	conditionalMu.Lock()
	defer conditionalMu.Unlock()
	propertyConditionals = nil
	profileConditionals = nil
	conditionalBeans = nil
}

// ExecuteConditionals evaluates and runs registered conditional callbacks and bean registrations.
func ExecuteConditionals(loader *ConfigLoader) {
	if loader != nil {
		SetActiveLoader(loader)
	}

	propActions, profActions, beans := snapshotConditionals()
	activeProfile := resolveActiveProfile(loader)

	executeProfileActions(profActions, activeProfile)
	executePropertyActions(propActions)
	executeConditionalBeans(beans)
}

func snapshotConditionals() ([]propertyAction, []profileAction, []conditionalBean) {
	conditionalMu.RLock()
	defer conditionalMu.RUnlock()

	propActions := make([]propertyAction, len(propertyConditionals))
	copy(propActions, propertyConditionals)

	profActions := make([]profileAction, len(profileConditionals))
	copy(profActions, profileConditionals)

	beans := make([]conditionalBean, len(conditionalBeans))
	copy(beans, conditionalBeans)

	return propActions, profActions, beans
}

func resolveActiveProfile(loader *ConfigLoader) string {
	if loader != nil && loader.ActiveProfile != "" {
		return loader.ActiveProfile
	}
	envProf := strings.TrimSpace(os.Getenv("SPRINGO_PROFILES_ACTIVE"))
	if envProf != "" {
		return envProf
	}
	return "default"
}

func executeProfileActions(actions []profileAction, activeProfile string) {
	for _, act := range actions {
		if matchesProfile(act.profiles, activeProfile) {
			act.fn()
		}
	}
}

func matchesProfile(profiles []string, activeProfile string) bool {
	for _, p := range profiles {
		if strings.EqualFold(strings.TrimSpace(p), activeProfile) {
			return true
		}
	}
	return false
}

func executePropertyActions(actions []propertyAction) {
	for _, act := range actions {
		if evaluateCondition(act.property, act.expected) {
			act.fn()
		}
	}
}

func executeConditionalBeans(beans []conditionalBean) {
	container := ioc.GetContainer()
	for _, b := range beans {
		if evaluateCondition(b.property, b.expected) {
			registerBeanInContainer(container, b.name, b.factory)
		}
	}
}

func registerBeanInContainer(container *ioc.ApplicationContainer, name string, factory any) {
	switch f := factory.(type) {
	case ioc.BeanFactory:
		container.RegisterServiceFactory(name, f)
	case func() interface{}:
		container.RegisterServiceFactory(name, f)
	case ioc.RepositoryFactory:
		container.RegisterRepositoryFactory(name, f)
	default:
		container.RegisterBean(name, factory)
	}
}

// EvaluateCondition checks if the configured property equals the expected value.
func evaluateCondition(property string, expected any) bool {
	actual, found := resolvePropertyPath(property)
	if !found || actual == nil {
		return expected == nil || expected == false || expected == ""
	}

	return compareValues(actual, expected)
}

func compareValues(actual, expected any) bool {
	if actual == expected {
		return true
	}

	if match, ok := compareBools(actual, expected); ok {
		return match
	}

	if match, ok := compareInts(actual, expected); ok {
		return match
	}

	return strings.EqualFold(coerceToString(actual), coerceToString(expected))
}

func compareBools(actual, expected any) (bool, bool) {
	expectedBool, ok := expected.(bool)
	if !ok {
		return false, false
	}
	actualBool, ok := coerceToBool(actual)
	if !ok {
		return false, true
	}
	return actualBool == expectedBool, true
}

func compareInts(actual, expected any) (bool, bool) {
	expectedInt, ok := coerceToInt(expected)
	if !ok {
		return false, false
	}
	actualInt, ok := coerceToInt(actual)
	if !ok {
		return false, true
	}
	return actualInt == expectedInt, true
}
