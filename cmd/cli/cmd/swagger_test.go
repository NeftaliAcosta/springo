package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSwaggerArgs(t *testing.T) {
	want := []string{
		"init",
		"-g", "main.go",
		"-d", "cmd/custom",
		"--parseInternal",
		"--pdl", "1",
		"--parseGoList=false",
		"-q",
	}
	if got := swaggerArgs("cmd/custom/main.go", true); !reflect.DeepEqual(got, want) {
		t.Fatalf("swaggerArgs() = %#v, want %#v", got, want)
	}
}

func TestCollectGoPackageDirs(t *testing.T) {
	root := t.TempDir()
	mustWriteTestFile(t, filepath.Join(root, "application", "service.go"))
	mustWriteTestFile(t, filepath.Join(root, "application", "service_test.go"))
	mustWriteTestFile(t, filepath.Join(root, "infrastructure", "rest", "controller.go"))
	mustWriteTestFile(t, filepath.Join(root, "README.md"))

	want := []string{filepath.Join(root, "application"), filepath.Join(root, "infrastructure", "rest")}
	if got := collectGoPackageDirs(root); !reflect.DeepEqual(got, want) {
		t.Fatalf("collectGoPackageDirs() = %#v, want %#v", got, want)
	}
}

func mustWriteTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package test"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSwaggerSearchConfigForRootMain(t *testing.T) {
	generalInfo, searchDirs := swaggerSearchConfig("main.go")
	if generalInfo != "main.go" || searchDirs != "." {
		t.Fatalf("swaggerSearchConfig() = %q, %q; want main.go, .", generalInfo, searchDirs)
	}
}
