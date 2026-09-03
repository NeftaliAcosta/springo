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

// Instantiate creates a new instance of a bean by invoking its factory function and running autowire.
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
	if len(results) == 0 || len(results) > 2 {
		return nil, fmt.Errorf(
			"factory for bean '%s' must return either T or (T, error), got %d return values",
			def.Name,
			len(results),
		)
	}

	if len(results) == 2 && results[1].Interface() != nil {
		if factoryErr, ok := results[1].Interface().(error); ok && factoryErr != nil {
			return nil, fmt.Errorf("factory for bean '%s' failed: %w", def.Name, factoryErr)
		}
	}

	bean := results[0].Interface()

	// Perform autowire and field injection
	if err := c.autowire(ctx, bean); err != nil {
		return nil, err
	}

	return bean, nil
}

// BuildFactoryArgs prepares parameter values for calling a bean factory function.
func (c *ApplicationContainer) buildFactoryArgs(
	ctx context.Context,
	factoryType reflect.Type,
	beanName string,
) ([]reflect.Value, error) {
	args := make([]reflect.Value, factoryType.NumIn())
	for i := 0; i < factoryType.NumIn(); i++ {
		paramType := factoryType.In(i)

		// 1. Direct match for *gorm.DB connection
		if paramType == reflect.TypeOf((*gorm.DB)(nil)) {
			if c.db != nil {
				args[i] = reflect.ValueOf(c.db)
				continue
			}
		}

		// 2. Direct match for context.Context interface
		if paramType.Implements(reflect.TypeOf((*context.Context)(nil)).Elem()) {
			args[i] = reflect.ValueOf(ctx)
			continue
		}

		// 3. Flexible resolution by registered bean type
		matchedName, err := c.findBeanNameByType(paramType)
		if err == nil {
			resolvedBean, err := c.GetBeanScoped(ctx, matchedName)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve dependency '%s' for bean '%s': %w", matchedName, beanName, err)
			}
			args[i] = reflect.ValueOf(resolvedBean)
			continue
		}

		return nil, fmt.Errorf(
			"unsupported or unresolvable factory parameter %d of type '%s' for bean '%s': %w",
			i,
			paramType,
			beanName,
			err,
		)
	}
	return args, nil
}

// FindBeanNameByType searches for a registered bean name whose type or factory return type is assignable to paramType.
func (c *ApplicationContainer) findBeanNameByType(paramType reflect.Type) (string, error) {
	matches := c.findMatchingBeanNames(paramType)

	if len(matches) == 0 {
		return "", fmt.Errorf("no bean of type '%s' found in container", paramType)
	}

	if len(matches) == 1 {
		return matches[0], nil
	}

	return selectBestNameMatch(matches, paramType)
}

// FindMatchingBeanNames collects all registered bean names assignable to the specified target type.
func (c *ApplicationContainer) findMatchingBeanNames(paramType reflect.Type) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var matches []string

	for name, def := range c.definitions {
		if isDefinitionAssignable(def, paramType) {
			matches = append(matches, name)
		}
	}

	for name, bean := range c.beans {
		if isBeanInstanceAssignable(bean, paramType, matches, name) {
			matches = append(matches, name)
		}
	}

	return matches
}

// IsDefinitionAssignable checks whether a bean definition's instance or factory return type matches paramType.
func isDefinitionAssignable(def *BeanDefinition, paramType reflect.Type) bool {
	if def.instance != nil && reflect.ValueOf(def.instance).Type().AssignableTo(paramType) {
		return true
	}

	if def.Factory == nil {
		return false
	}

	factoryVal := reflect.ValueOf(def.Factory)
	factoryType := factoryVal.Type()
	if factoryType.Kind() != reflect.Func || factoryType.NumOut() == 0 {
		return false
	}

	return factoryType.Out(0).AssignableTo(paramType)
}

// IsBeanInstanceAssignable checks whether a cached bean instance is assignable to paramType and not already matched.
func isBeanInstanceAssignable(bean interface{}, paramType reflect.Type, existing []string, name string) bool {
	if bean == nil {
		return false
	}

	for _, matchedName := range existing {
		if matchedName == name {
			return false
		}
	}

	return reflect.ValueOf(bean).Type().AssignableTo(paramType)
}

// SelectBestNameMatch selects the most specific bean name match when multiple candidates are registered.
func selectBestNameMatch(matches []string, paramType reflect.Type) (string, error) {
	paramTypeName := paramType.Name()
	if paramType.Kind() == reflect.Pointer {
		paramTypeName = paramType.Elem().Name()
	}

	for _, matchName := range matches {
		if strings.EqualFold(matchName, paramTypeName) {
			return matchName, nil
		}
	}

	return "", fmt.Errorf("ambiguous dependency for type '%s': multiple matching beans found %v", paramType, matches)
}

// Autowire inspects struct fields marked with `spring` tags and injects matching dependencies.
func (c *ApplicationContainer) autowire(ctx context.Context, bean interface{}) error {
	val := reflect.ValueOf(bean)
	if val.Kind() == reflect.Pointer {
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

// InjectDependencyField sets either a Provider instance or a resolved bean dependency into a target field.
func (c *ApplicationContainer) injectDependencyField(ctx context.Context, fieldVal reflect.Value, field reflect.StructField, tag string) error {
	if isProviderType(field.Type) {
		targetType := field.Type
		isPtr := targetType.Kind() == reflect.Pointer
		structType := targetType
		if isPtr {
			structType = targetType.Elem()
		}

		providerStruct := reflect.New(structType).Elem()
		providerStruct.FieldByName("Container").Set(reflect.ValueOf(c))
		providerStruct.FieldByName("BeanName").Set(reflect.ValueOf(tag))

		if isPtr {
			fieldVal.Set(providerStruct.Addr())
		} else {
			fieldVal.Set(providerStruct)
		}
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

// IsProviderType checks if a type or pointer to type matches the Provider[T] structural contract:
// Containing fields Container (*ApplicationContainer), BeanName (string), and method Get(context.Context) (T, error).
func isProviderType(t reflect.Type) bool {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return false
	}

	containerField, hasContainer := t.FieldByName("Container")
	if !hasContainer || containerField.Type != reflect.TypeOf((*ApplicationContainer)(nil)) {
		return false
	}

	beanNameField, hasBeanName := t.FieldByName("BeanName")
	if !hasBeanName || beanNameField.Type.Kind() != reflect.String {
		return false
	}

	getMethod, hasGet := t.MethodByName("Get")
	if !hasGet || getMethod.Type.NumIn() != 2 || getMethod.Type.NumOut() != 2 {
		return false
	}

	ctxType := reflect.TypeOf((*context.Context)(nil)).Elem()
	errType := reflect.TypeOf((*error)(nil)).Elem()
	if getMethod.Type.In(1) != ctxType || getMethod.Type.Out(1) != errType {
		return false
	}

	return true
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
	for _, def := range c.definitions {
		def.instance = nil
		def.err = nil
		def.once = sync.Once{}
	}
	c.db = nil
}

// ResetAll removes all initialized bean instances, resets the DB connection,
// and unregisters all bean definitions and factories (full container reset).
func (c *ApplicationContainer) ResetAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.beans = make(map[string]interface{})
	c.definitions = make(map[string]*BeanDefinition)
	c.repositoryFactories = make(map[string]RepositoryFactory)
	c.serviceFactories = make(map[string]BeanFactory)
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
