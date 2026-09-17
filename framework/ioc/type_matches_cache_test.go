package ioc

import (
	"reflect"
	"testing"
)

type cachedDependency interface {
	Value() string
}

type cachedDependencyImpl struct{}

func (cachedDependencyImpl) Value() string { return "ok" }

func TestFindMatchingBeanNamesInvalidatesAfterRegistration(t *testing.T) {
	container := GetContainer()
	container.ResetAll()
	targetType := reflect.TypeOf((*cachedDependency)(nil)).Elem()

	if matches := container.findMatchingBeanNames(targetType); len(matches) != 0 {
		t.Fatalf("expected empty cache before registration, got %v", matches)
	}
	container.RegisterBean("cached-dependency", cachedDependencyImpl{})
	matches := container.findMatchingBeanNames(targetType)
	if len(matches) != 1 || matches[0] != "cached-dependency" {
		t.Fatalf("expected registered dependency after cache invalidation, got %v", matches)
	}
}
