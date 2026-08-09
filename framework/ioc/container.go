package ioc

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"

	"gorm.io/gorm"
)

type Scope string

const (
	ScopeSingleton Scope = "singleton"
	ScopePrototype Scope = "prototype"
	ScopeRequest   Scope = "request"
)

// contextKey is a private type to prevent key collisions in context.Context
type contextKey struct{}

var registryKey = contextKey{}

// RegistryKey returns the key used to store RequestRegistry in context.Context
func RegistryKey() interface{} {
	return registryKey
}

type resolutionStackKey struct{}

// BeanFactory is a function that creates a bean instance
type BeanFactory func() interface{}

// RepositoryFactory is a function that creates a repository using a DB connection
type RepositoryFactory func(db *gorm.DB) interface{}

// BeanDefinition holds the construction metadata for a bean
type BeanDefinition struct {
	Name         string
	Scope        Scope
	Factory      interface{} // Can be BeanFactory, RepositoryFactory, or func(context.Context) interface{}
	Dependencies []string
	once         sync.Once
	instance     interface{}
	err          error
}

// RequestRegistry holds instances created specifically for a single HTTP request context
type RequestRegistry struct {
	instances sync.Map
	container *ApplicationContainer
}

// Set stores a bean instance in the request registry
func (r *RequestRegistry) Set(name string, val interface{}) {
	r.instances.Store(name, val)
}

// Get retrieves a bean instance from the request registry
func (r *RequestRegistry) Get(name string) (interface{}, bool) {
	return r.instances.Load(name)
}

// Provider provides lazy/on-demand retrieval of scoped beans
type Provider[T any] struct {
	Container *ApplicationContainer
	BeanName  string
}

// Get resolves the bean instance dynamically using the given context
func (p Provider[T]) Get(ctx context.Context) (T, error) {
	var zero T
	instance, err := p.Container.GetBeanScoped(ctx, p.BeanName)
	if err != nil {
		return zero, err
	}
	typed, ok := instance.(T)
	if !ok {
		return zero, fmt.Errorf("bean '%s' is of type %T, expected %T", p.BeanName, instance, zero)
	}
	return typed, nil
}

// ApplicationContainer holds all initialized beans (Spring ApplicationContext)
type ApplicationContainer struct {
	beans               map[string]interface{}
	definitions         map[string]*BeanDefinition
	repositoryFactories map[string]RepositoryFactory
	serviceFactories    map[string]BeanFactory
	db                  *gorm.DB
	mu                  sync.RWMutex
}

var (
	instance *ApplicationContainer
	once     sync.Once
)

// GetContainer returns the singleton instance of the IoC container
func GetContainer() *ApplicationContainer {
	once.Do(func() {
		instance = &ApplicationContainer{
			beans:               make(map[string]interface{}),
			definitions:         make(map[string]*BeanDefinition),
			repositoryFactories: make(map[string]RepositoryFactory),
			serviceFactories:    make(map[string]BeanFactory),
		}
	})
	return instance
}

// Option allows configuring BeanDefinitions during registration
type Option func(*BeanDefinition)

// WithScope configures the scope of a bean
func WithScope(scope Scope) Option {
	return func(d *BeanDefinition) {
		d.Scope = scope
	}
}

// RegisterBeanDefinition registers construction metadata for a bean
func (c *ApplicationContainer) RegisterBeanDefinition(name string, factory interface{}, opts ...Option) {
	c.mu.Lock()
	defer c.mu.Unlock()

	def := &BeanDefinition{
		Name:    name,
		Scope:   ScopeSingleton, // Default
		Factory: factory,
	}

	for _, opt := range opts {
		opt(def)
	}

	c.definitions[name] = def
}

// RegisterRepositoryFactory adds a repository factory to be initialized later
func (c *ApplicationContainer) RegisterRepositoryFactory(name string, factory RepositoryFactory) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.repositoryFactories[name] = factory
	c.definitions[name] = &BeanDefinition{
		Name:    name,
		Scope:   ScopeSingleton,
		Factory: factory,
	}
}

// RegisterServiceFactory adds a service factory to be initialized later
func (c *ApplicationContainer) RegisterServiceFactory(name string, factory BeanFactory) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.serviceFactories[name] = factory
	c.definitions[name] = &BeanDefinition{
		Name:    name,
		Scope:   ScopeSingleton,
		Factory: factory,
	}
}

// InitializeAllBeans executes all factories in the correct order (Singleton scope only)
func (c *ApplicationContainer) InitializeAllBeans(db *gorm.DB) {
	c.db = db

	// 1. Initialize Repositories (Legacy factories default to Singleton)
	for name, factory := range c.repositoryFactories {
		def := c.definitions[name]
		if def == nil || def.Scope == ScopeSingleton {
			bean := factory(db)
			c.RegisterBean(name, bean)
		}
	}

	// 2. Initialize Services
	for name, factory := range c.serviceFactories {
		def := c.definitions[name]
		if def == nil || def.Scope == ScopeSingleton {
			bean := factory()
			c.RegisterBean(name, bean)
		}
	}
}

// RegisterBean adds a new instance to the container
func (c *ApplicationContainer) RegisterBean(name string, bean interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.beans[name] = bean

	// Ensure there is a definition matching it so GetBeanScoped is aware of it
	if _, exists := c.definitions[name]; !exists {
		c.definitions[name] = &BeanDefinition{
			Name:     name,
			Scope:    ScopeSingleton,
			instance: bean,
		}
		c.definitions[name].once.Do(func() {
			// Empty: Used solely to mark the sync.Once initialization as completed
		}) // Mark once as completed
	}
}

// GetBean retrieves a bean by its name (context-less fallback)
func (c *ApplicationContainer) GetBean(name string) interface{} {
	bean, err := c.GetBeanScoped(context.Background(), name)
	if err != nil {
		return nil
	}
	return bean
}

// GetBeanScoped retrieves a bean by its name with support for scopes and context resolution
func (c *ApplicationContainer) GetBeanScoped(ctx context.Context, name string) (interface{}, error) {
	// First check singleton cache
	c.mu.RLock()
	if bean, ok := c.beans[name]; ok {
		c.mu.RUnlock()
		return bean, nil
	}

	def, exists := c.definitions[name]
	c.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("bean '%s' is not registered in the container", name)
	}

	// Get resolution stack from context
	var stack []string
	if val := ctx.Value(resolutionStackKey{}); val != nil {
		stack = val.([]string)
	}

	// Detect circular dependencies
	if err := checkCircularDependency(stack, name); err != nil {
		return nil, err
	}

	// Push current bean to stack
	newCtx := context.WithValue(ctx, resolutionStackKey{}, append(stack, name))

	switch def.Scope {
	case ScopeSingleton:
		return c.resolveSingletonBean(newCtx, def)

	case ScopePrototype:
		return c.instantiate(newCtx, def)

	case ScopeRequest:
		return c.resolveRequestBean(newCtx, def)

	default:
		return nil, fmt.Errorf("unsupported scope '%s' for bean '%s'", def.Scope, name)
	}
}

func checkCircularDependency(stack []string, name string) error {
	for _, s := range stack {
		if s == name {
			return fmt.Errorf("circular dependency detected resolving bean '%s'", name)
		}
	}
	return nil
}

func (c *ApplicationContainer) resolveSingletonBean(ctx context.Context, def *BeanDefinition) (interface{}, error) {
	def.once.Do(func() {
		def.instance, def.err = c.instantiate(ctx, def)
		if def.err == nil {
			c.mu.Lock()
			c.beans[def.Name] = def.instance
			c.mu.Unlock()
		}
	})
	return def.instance, def.err
}

func (c *ApplicationContainer) resolveRequestBean(ctx context.Context, def *BeanDefinition) (interface{}, error) {
	registry, ok := ctx.Value(registryKey).(*RequestRegistry)
	if !ok || registry == nil {
		return nil, fmt.Errorf("scope error: tried to resolve RequestScope bean '%s' outside of an HTTP request context", def.Name)
	}

	if val, found := registry.instances.Load(def.Name); found {
		return val, nil
	}

	// Instantiate new instance for this HTTP request
	instance, err := c.instantiate(ctx, def)
	if err != nil {
		return nil, err
	}

	registry.instances.Store(def.Name, instance)
	return instance, nil
}

func (c *ApplicationContainer) instantiate(ctx context.Context, def *BeanDefinition) (interface{}, error) {
	factoryVal := reflect.ValueOf(def.Factory)
	factoryType := factoryVal.Type()

	if factoryType.Kind() != reflect.Func {
		return nil, fmt.Errorf("factory for bean '%s' is not a function", def.Name)
	}

	args, err := c.buildFactoryArgs(ctx, factoryType, def.Name)
	if err != nil {
		return nil, err
	}

	results := factoryVal.Call(args)
	if len(results) == 0 {
		return nil, fmt.Errorf("factory for bean '%s' returned no values", def.Name)
	}

	bean := results[0].Interface()

	// Perform autowire and field injection
	if err := c.autowire(ctx, bean); err != nil {
		return nil, err
	}

	return bean, nil
}

func (c *ApplicationContainer) buildFactoryArgs(ctx context.Context, factoryType reflect.Type, beanName string) ([]reflect.Value, error) {
	args := make([]reflect.Value, factoryType.NumIn())
	for i := 0; i < factoryType.NumIn(); i++ {
		paramType := factoryType.In(i)

		if paramType == reflect.TypeOf((*gorm.DB)(nil)) {
			args[i] = reflect.ValueOf(c.db)
			continue
		}

		if paramType.Implements(reflect.TypeOf((*context.Context)(nil)).Elem()) {
			args[i] = reflect.ValueOf(ctx)
			continue
		}

		return nil, fmt.Errorf("unsupported factory parameter type '%s' for bean '%s'", paramType, beanName)
	}
	return args, nil
}

func (c *ApplicationContainer) autowire(ctx context.Context, bean interface{}) error {
	val := reflect.ValueOf(bean)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return nil
	}

	for i := 0; i < val.NumField(); i++ {
		field := val.Type().Field(i)
		tag := field.Tag.Get("spring")
		if tag == "" {
			continue
		}

		fieldVal := val.Field(i)
		if !fieldVal.CanSet() {
			continue
		}

		if err := c.injectDependencyField(ctx, fieldVal, field, tag); err != nil {
			return err
		}
	}

	return nil
}

func (c *ApplicationContainer) injectDependencyField(ctx context.Context, fieldVal reflect.Value, field reflect.StructField, tag string) error {
	if isProviderType(field.Type) {
		provider := reflect.New(field.Type).Elem()
		provider.FieldByName("Container").Set(reflect.ValueOf(c))
		provider.FieldByName("BeanName").Set(reflect.ValueOf(tag))
		fieldVal.Set(provider)
		return nil
	}

	dependency, err := c.GetBeanScoped(ctx, tag)
	if err != nil {
		return fmt.Errorf("failed to inject dependency '%s' in field '%s': %w", tag, field.Name, err)
	}
	dependencyVal := reflect.ValueOf(dependency)
	if !dependencyVal.Type().AssignableTo(field.Type) {
		return fmt.Errorf("dependency '%s' of type %T is not assignable to field '%s' of type %s", tag, dependency, field.Name, field.Type)
	}
	fieldVal.Set(dependencyVal)
	return nil
}

func isProviderType(t reflect.Type) bool {
	return t.Kind() == reflect.Struct &&
		strings.HasPrefix(t.Name(), "Provider[") &&
		strings.HasSuffix(t.Name(), "]") &&
		t.PkgPath() == "github.com/NeftaliAcosta/springo/framework/ioc"
}

// CreateRequestRegistry instantiates a request scope bean container registry
func (c *ApplicationContainer) CreateRequestRegistry() *RequestRegistry {
	return &RequestRegistry{
		container: c,
	}
}

// DestroyRequestRegistry cleans up all request scope beans and runs io.Closer if implemented
func (c *ApplicationContainer) DestroyRequestRegistry(r *RequestRegistry) {
	if r == nil {
		return
	}
	r.instances.Range(func(key, value interface{}) bool {
		if closer, ok := value.(io.Closer); ok {
			_ = closer.Close()
		}
		r.instances.Delete(key)
		return true
	})
}

// ReplaceBean overwrites an existing bean (Useful for Mocking in Tests)
func (c *ApplicationContainer) ReplaceBean(name string, mockBean interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.beans[name] = mockBean

	if _, exists := c.definitions[name]; !exists {
		c.definitions[name] = &BeanDefinition{
			Name:     name,
			Scope:    ScopeSingleton,
			instance: mockBean,
		}
		c.definitions[name].once.Do(func() {
			// Empty: Used solely to mark the sync.Once initialization as completed
		})
	} else {
		c.definitions[name].instance = mockBean
	}
}

// SetDB forcefully replaces the primary DB connection (Useful for Tests)
func (c *ApplicationContainer) SetDB(db *gorm.DB) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.db = db
}

// Clear removes all initialized bean instances and resets the DB connection,
// but preserves the registered factories so they can be re-initialized.
func (c *ApplicationContainer) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.beans = make(map[string]interface{})
	c.definitions = make(map[string]*BeanDefinition)
	c.db = nil
}

// GetAllBeans returns a copy of all registered beans
func (c *ApplicationContainer) GetAllBeans() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	copy := make(map[string]interface{})
	for k, v := range c.beans {
		copy[k] = v
	}
	return copy
}

// GetDB retrieves the database connection
func (c *ApplicationContainer) GetDB() *gorm.DB {
	c.mu.RLock()
	if c.db != nil {
		defer c.mu.RUnlock()
		return c.db
	}
	if bean, ok := c.beans["DB"]; ok {
		if db, ok := bean.(*gorm.DB); ok {
			c.mu.RUnlock()
			return db
		}
	}
	c.mu.RUnlock()
	return nil
}
