package ioc

import "testing"

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
