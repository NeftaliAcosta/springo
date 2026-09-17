package cache

import (
	"context"
	"testing"
)

func BenchmarkMemoryCacheGetSet(b *testing.B) {
	ctx := context.Background()
	memory := (&memoryProvider{}).GetCache("benchmark", 0)
	_ = memory.Set(ctx, "key", "value", 0)
	b.ReportAllocs()
	for b.Loop() {
		if err := memory.Set(ctx, "key", "value", 0); err != nil {
			b.Fatal(err)
		}
		if _, ok := memory.Get(ctx, "key"); !ok {
			b.Fatal("cache value missing")
		}
	}
}
