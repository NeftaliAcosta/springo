package config

import (
	"github.com/NeftaliAcosta/springo/framework/ioc"
	"reflect"
	"sync"
)

// PropertyRegistry holds the configuration templates to be filled on startup
type PropertyRegistry struct {
	prefix string
	target interface{}
}

var (
	registry     []PropertyRegistry
	defaultsOnce sync.Once
	defaultsMap  map[interface{}]reflect.Value
	configMu     sync.Mutex
)

// deepClone recursively creates a deep copy of a reflect.Value
func deepClone(v reflect.Value) reflect.Value {
	if !v.IsValid() {
		return reflect.Value{}
	}

	switch v.Kind() {
	case reflect.Ptr:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		val := reflect.New(v.Type().Elem())
		val.Elem().Set(deepClone(v.Elem()))
		return val

	case reflect.Slice:
		return cloneSlice(v)

	case reflect.Map:
		return cloneMap(v)

	case reflect.Struct:
		return cloneStruct(v)

	case reflect.Interface:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		concrete := deepClone(v.Elem())
		val := reflect.New(v.Type()).Elem()
		val.Set(concrete)
		return val

	default:
		val := reflect.New(v.Type()).Elem()
		val.Set(v)
		return val
	}
}

func cloneSlice(v reflect.Value) reflect.Value {
	if v.IsNil() {
		return reflect.Zero(v.Type())
	}
	val := reflect.MakeSlice(v.Type(), v.Len(), v.Cap())
	for i := 0; i < v.Len(); i++ {
		val.Index(i).Set(deepClone(v.Index(i)))
	}
	return val
}

func cloneMap(v reflect.Value) reflect.Value {
	if v.IsNil() {
		return reflect.Zero(v.Type())
	}
	val := reflect.MakeMap(v.Type())
	iter := v.MapRange()
	for iter.Next() {
		val.SetMapIndex(deepClone(iter.Key()), deepClone(iter.Value()))
	}
	return val
}

func cloneStruct(v reflect.Value) reflect.Value {
	val := reflect.New(v.Type()).Elem()
	for i := 0; i < v.NumField(); i++ {
		f := val.Field(i)
		if f.CanSet() {
			f.Set(deepClone(v.Field(i)))
		}
	}
	return val
}

// RegisterProperties registers a struct to be automatically filled from YAML with a prefix
func RegisterProperties(prefix string, target interface{}) {
	configMu.Lock()
	defer configMu.Unlock()
	registry = append(registry, PropertyRegistry{
		prefix: prefix,
		target: target,
	})

	// defaultsOnce may already have run (for example in repeated tests or a
	// dynamic extension). Capture the new target immediately so ResetProperties
	// remains correct for every registered property, not only the initial set.
	if defaultsMap != nil {
		orig := reflect.ValueOf(target).Elem()
		defaultsMap[target] = deepClone(orig)
	}
}

// ResetProperties restores all registered targets to their captured default values using deep clone
func ResetProperties() {
	configMu.Lock()
	defer configMu.Unlock()
	if defaultsMap == nil {
		return
	}
	for target, clone := range defaultsMap {
		reflect.ValueOf(target).Elem().Set(deepClone(clone))
	}
}

// Validatable defines an interface for configuration properties that require validation after binding.
type Validatable interface {
	Validate() error
}

// InitializeProperties fills all registered properties from the loader, validates them, and registers them in IoC
func InitializeProperties(loader *ConfigLoader) error {
	configMu.Lock()
	defer configMu.Unlock()
	defaultsOnce.Do(func() {
		defaultsMap = make(map[interface{}]reflect.Value)
		for _, reg := range registry {
			orig := reflect.ValueOf(reg.target).Elem()
			defaultsMap[reg.target] = deepClone(orig)
		}
	})

	// Binding mutates registered targets. Keep it in the same critical section
	// as ResetProperties so initialization and reset cannot race on those
	// objects.
	for _, reg := range registry {
		if err := loader.BindPrefix(reg.prefix, reg.target); err != nil {
			return err
		}
		// Validate properties if they implement Validatable interface
		if v, ok := reg.target.(Validatable); ok {
			if err := v.Validate(); err != nil {
				return err
			}
		}
		// Register the filled struct as a Bean in IoC container
		// We use the pointer type name as the bean name
		t := reflect.TypeOf(reg.target).Elem()
		fullName := t.PkgPath() + "." + t.Name()
		ioc.GetContainer().RegisterBean(fullName, reg.target)
		ioc.GetContainer().RegisterBean(t.Name(), reg.target)
	}
	return nil
}

// Get retrieves a registered property bean from the IoC container
func Get[T any]() *T {
	var zero T
	t := reflect.TypeOf(zero)
	fullName := t.PkgPath() + "." + t.Name()
	bean := ioc.GetContainer().GetBean(fullName)
	if bean == nil {
		bean = ioc.GetContainer().GetBean(t.Name())
	}
	if bean == nil {
		return nil
	}
	res, ok := bean.(*T)
	if !ok {
		return nil
	}
	return res
}

// GetConfigProperties returns a copy of all loaded properties grouped by their prefix
func GetConfigProperties() map[string]interface{} {
	configMu.Lock()
	defer configMu.Unlock()
	props := make(map[string]interface{})
	for _, reg := range registry {
		props[reg.prefix] = reg.target
	}
	return props
}
