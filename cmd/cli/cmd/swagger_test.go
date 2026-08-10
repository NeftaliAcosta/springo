package cmd

import (
	"reflect"
	"testing"
)

func TestSwaggerArgs(t *testing.T) {
	want := []string{
		"init",
		"-g", "cmd/custom/main.go",
		"--parseInternal",
		"--pdl", "1",
		"--parseGoList=false",
		"-q",
	}
	if got := swaggerArgs("cmd/custom/main.go", true); !reflect.DeepEqual(got, want) {
		t.Fatalf("swaggerArgs() = %#v, want %#v", got, want)
	}
}
