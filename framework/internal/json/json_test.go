package json

import (
	"bytes"
	stdjson "encoding/json"
	"reflect"
	"testing"
)

func TestEngineParity(t *testing.T) {
	value := struct {
		Name  string `json:"name"`
		Items []int  `json:"items"`
	}{Name: "springo", Items: []int{1, 2, 3}}
	standardBytes, err := standardCodec{}.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	goccyBytes, err := goccyCodec{}.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var standardValue, goccyValue map[string]any
	if err := stdjson.Unmarshal(standardBytes, &standardValue); err != nil {
		t.Fatal(err)
	}
	if err := stdjson.Unmarshal(goccyBytes, &goccyValue); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(standardValue, goccyValue) {
		t.Fatalf("semantic mismatch: standard=%s goccy=%s", standardBytes, goccyBytes)
	}
}

func TestEngineSelection(t *testing.T) {
	var standard bytes.Buffer
	if err := NewEncoderFor("standard", &standard).Encode(map[string]int{"value": 1}); err != nil {
		t.Fatal(err)
	}
	var goccy bytes.Buffer
	if err := NewEncoderFor("go-json", &goccy).Encode(map[string]int{"value": 1}); err != nil {
		t.Fatal(err)
	}
	if standard.String() != goccy.String() {
		t.Fatalf("engine output mismatch: standard=%q goccy=%q", standard.String(), goccy.String())
	}
}
