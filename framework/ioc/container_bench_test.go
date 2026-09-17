package ioc

import (
	"reflect"
	"testing"
)

type benchmarkService interface {
	Run()
}

type benchmarkServiceImpl struct{}

func (benchmarkServiceImpl) Run() {}

func BenchmarkGetBean(b *testing.B) {
	container := GetContainer()
	container.RegisterBean("benchmark-bean", struct{ Value int }{Value: 1})
	b.ReportAllocs()
	for b.Loop() {
		if container.GetBean("benchmark-bean") == nil {
			b.Fatal("benchmark bean missing")
		}
	}
}

func BenchmarkFindBeanNameByType(b *testing.B) {
	container := GetContainer()
	container.ResetAll()
	container.RegisterBean("benchmark-service", benchmarkServiceImpl{})
	targetType := reflect.TypeOf((*benchmarkService)(nil)).Elem()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := container.findBeanNameByType(targetType); err != nil {
			b.Fatal(err)
		}
	}
}
