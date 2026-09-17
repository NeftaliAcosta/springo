package json

import (
	"bytes"
	"testing"
)

func BenchmarkStandardMarshal(b *testing.B) {
	value := map[string]any{"status": 200, "message": "ok", "items": []int{1, 2, 3}}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := (standardCodec{}).Marshal(value); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGoccyMarshal(b *testing.B) {
	value := map[string]any{"status": 200, "message": "ok", "items": []int{1, 2, 3}}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := (goccyCodec{}).Marshal(value); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStandardUnmarshal(b *testing.B) {
	data := []byte(`{"status":200,"message":"ok","items":[1,2,3]}`)
	b.ReportAllocs()
	for b.Loop() {
		var value map[string]any
		if err := (standardCodec{}).Unmarshal(data, &value); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGoccyUnmarshal(b *testing.B) {
	data := []byte(`{"status":200,"message":"ok","items":[1,2,3]}`)
	b.ReportAllocs()
	for b.Loop() {
		var value map[string]any
		if err := (goccyCodec{}).Unmarshal(data, &value); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncoderSelection(b *testing.B) {
	value := map[string]int{"status": 200}
	b.ReportAllocs()
	for b.Loop() {
		var buffer bytes.Buffer
		if err := NewEncoderFor("go-json", &buffer).Encode(value); err != nil {
			b.Fatal(err)
		}
	}
}
