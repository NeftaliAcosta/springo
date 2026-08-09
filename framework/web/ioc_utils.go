package web

import (
	"github.com/NeftaliAcosta/springo/framework/ioc"
)

// GetService retrieves a service from the IoC container and casts it to the desired type
func GetService[T any](name string) T {
	bean := ioc.GetContainer().GetBean(name)
	if bean == nil {
		var zero T
		return zero
	}
	return bean.(T)
}
