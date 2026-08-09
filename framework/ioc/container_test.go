package ioc

import (
	"context"
	"strings"
	"sync"
	"testing"
)

type SingletonBean struct {
	Value string
}

type PrototypeBean struct {
	Value string
}

type RequestBean struct {
	Value string
}

type CircularA struct {
	B *CircularB `spring:"CircularB"`
}

type CircularB struct {
	A *CircularA `spring:"CircularA"`
}

type ComponentWithProvider struct {
	RequestBeanProvider Provider[*RequestBean] `spring:"RequestBean"`
}

type ComponentWithDirectInjection struct {
	Singleton *SingletonBean `spring:"SingletonBean"`
}

func TestContainer_Scopes(t *testing.T) {
	c := GetContainer()
	c.Clear()

	// Register definitions
	c.RegisterBeanDefinition("SingletonBean", func() *SingletonBean {
		return &SingletonBean{Value: "singleton"}
	}, WithScope(ScopeSingleton))

	c.RegisterBeanDefinition("PrototypeBean", func() *PrototypeBean {
		return &PrototypeBean{Value: "prototype"}
	}, WithScope(ScopePrototype))

	c.RegisterBeanDefinition("RequestBean", func() *RequestBean {
		return &RequestBean{Value: "request"}
	}, WithScope(ScopeRequest))

	// 1. Test Singleton: always the same instance
	s1, err := c.GetBeanScoped(context.Background(), "SingletonBean")
	if err != nil {
		t.Fatalf("failed to resolve SingletonBean: %v", err)
	}
	s2, err := c.GetBeanScoped(context.Background(), "SingletonBean")
	if err != nil {
		t.Fatalf("failed to resolve SingletonBean: %v", err)
	}
	if s1 != s2 {
		t.Errorf("expected same instance for Singleton, got different pointers")
	}

	// 2. Test Prototype: different instances
	p1, err := c.GetBeanScoped(context.Background(), "PrototypeBean")
	if err != nil {
		t.Fatalf("failed to resolve PrototypeBean: %v", err)
	}
	p2, err := c.GetBeanScoped(context.Background(), "PrototypeBean")
	if err != nil {
		t.Fatalf("failed to resolve PrototypeBean: %v", err)
	}
	if p1 == p2 {
		t.Errorf("expected different instances for Prototype, got same pointer")
	}

	// 3. Test Request: fails outside request context
	_, err = c.GetBeanScoped(context.Background(), "RequestBean")
	if err == nil {
		t.Errorf("expected error when resolving Request scope bean outside request context, got nil")
	}

	// Create registries
	reg1 := c.CreateRequestRegistry()
	defer c.DestroyRequestRegistry(reg1)

	ctx1 := context.WithValue(context.Background(), registryKey, reg1)

	// Resolve in context 1
	r1_1, err := c.GetBeanScoped(ctx1, "RequestBean")
	if err != nil {
		t.Fatalf("failed to resolve RequestBean in ctx1: %v", err)
	}
	r1_2, err := c.GetBeanScoped(ctx1, "RequestBean")
	if err != nil {
		t.Fatalf("failed to resolve RequestBean in ctx1: %v", err)
	}
	if r1_1 != r1_2 {
		t.Errorf("expected same instance within same request context, got different pointers")
	}

	// Resolve in context 2
	reg2 := c.CreateRequestRegistry()
	defer c.DestroyRequestRegistry(reg2)
	ctx2 := context.WithValue(context.Background(), registryKey, reg2)

	r2_1, err := c.GetBeanScoped(ctx2, "RequestBean")
	if err != nil {
		t.Fatalf("failed to resolve RequestBean in ctx2: %v", err)
	}
	if r1_1 == r2_1 {
		t.Errorf("expected different instances between different request contexts, got same pointer")
	}
}

func TestContainer_Provider(t *testing.T) {
	c := GetContainer()
	c.Clear()

	c.RegisterBeanDefinition("RequestBean", func() *RequestBean {
		return &RequestBean{}
	}, WithScope(ScopeRequest))

	c.RegisterBeanDefinition("ComponentWithProvider", func() *ComponentWithProvider {
		return &ComponentWithProvider{}
	}, WithScope(ScopeSingleton))

	// Instantiate Singleton component
	compBean, err := c.GetBeanScoped(context.Background(), "ComponentWithProvider")
	if err != nil {
		t.Fatalf("failed to resolve ComponentWithProvider: %v", err)
	}

	comp := compBean.(*ComponentWithProvider)

	// Context 1
	reg1 := c.CreateRequestRegistry()
	defer c.DestroyRequestRegistry(reg1)
	ctx1 := context.WithValue(context.Background(), registryKey, reg1)

	req1, err := comp.RequestBeanProvider.Get(ctx1)
	if err != nil {
		t.Fatalf("failed to resolve RequestBean via provider: %v", err)
	}
	req1.Value = "hello-ctx1"

	// Check it was stored in ctx1 registry
	req1_again, _ := comp.RequestBeanProvider.Get(ctx1)
	if req1_again.Value != "hello-ctx1" {
		t.Errorf("expected value 'hello-ctx1', got '%s'", req1_again.Value)
	}

	// Context 2
	reg2 := c.CreateRequestRegistry()
	defer c.DestroyRequestRegistry(reg2)
	ctx2 := context.WithValue(context.Background(), registryKey, reg2)

	req2, err := comp.RequestBeanProvider.Get(ctx2)
	if err != nil {
		t.Fatalf("failed to resolve RequestBean via provider in ctx2: %v", err)
	}
	if req2.Value == "hello-ctx1" {
		t.Errorf("expected empty value or different instance in ctx2, got 'hello-ctx1'")
	}
}

func TestContainer_DirectInjection(t *testing.T) {
	c := GetContainer()
	c.Clear()

	c.RegisterBeanDefinition("SingletonBean", func() *SingletonBean {
		return &SingletonBean{Value: "injected-singleton"}
	}, WithScope(ScopeSingleton))

	c.RegisterBeanDefinition("ComponentWithDirect", func() *ComponentWithDirectInjection {
		return &ComponentWithDirectInjection{}
	}, WithScope(ScopeSingleton))

	compBean, err := c.GetBeanScoped(context.Background(), "ComponentWithDirect")
	if err != nil {
		t.Fatalf("failed to resolve ComponentWithDirect: %v", err)
	}

	comp := compBean.(*ComponentWithDirectInjection)
	if comp.Singleton == nil {
		t.Fatalf("expected Singleton field to be injected, got nil")
	}
	if comp.Singleton.Value != "injected-singleton" {
		t.Errorf("expected injected singleton value to be 'injected-singleton', got '%s'", comp.Singleton.Value)
	}
}

func TestContainer_CircularDependency(t *testing.T) {
	c := GetContainer()
	c.Clear()

	c.RegisterBeanDefinition("CircularA", func() *CircularA {
		return &CircularA{}
	}, WithScope(ScopePrototype))

	c.RegisterBeanDefinition("CircularB", func() *CircularB {
		return &CircularB{}
	}, WithScope(ScopePrototype))

	_, err := c.GetBeanScoped(context.Background(), "CircularA")
	if err == nil {
		t.Fatalf("expected circular dependency error, got nil")
	}
	if !strings.Contains(err.Error(), "circular dependency") {
		t.Errorf("expected circular dependency error message, got: %v", err)
	}
}

func TestContainer_Concurrency(t *testing.T) {
	c := GetContainer()
	c.Clear()

	c.RegisterBeanDefinition("RequestBean", func() *RequestBean {
		return &RequestBean{}
	}, WithScope(ScopeRequest))

	var wg sync.WaitGroup
	numRequests := 100

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			reg := c.CreateRequestRegistry()
			defer c.DestroyRequestRegistry(reg)
			ctx := context.WithValue(context.Background(), registryKey, reg)

			// Resolve
			beanInterface, err := c.GetBeanScoped(ctx, "RequestBean")
			if err != nil {
				t.Errorf("[Routine %d] failed to resolve: %v", id, err)
				return
			}
			bean := beanInterface.(*RequestBean)
			bean.Value = string(rune(id))

			// Resolve again in same routine, must be identical
			bean2, _ := c.GetBeanScoped(ctx, "RequestBean")
			if bean2.(*RequestBean).Value != string(rune(id)) {
				t.Errorf("[Routine %d] got mismatched instance or race condition", id)
			}
		}(i)
	}

	wg.Wait()
}
