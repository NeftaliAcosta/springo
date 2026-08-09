package web

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/NeftaliAcosta/springo/framework/version"
)

func TestShowBannerUsesCanonicalBrandAndVersion(t *testing.T) {
	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	t.Cleanup(func() {
		os.Stdout = originalStdout
		_ = reader.Close()
		_ = writer.Close()
	})

	os.Stdout = writer
	ShowBanner()
	if err := writer.Close(); err != nil {
		t.Fatalf("close stdout writer: %v", err)
	}
	os.Stdout = originalStdout

	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read banner output: %v", err)
	}
	text := string(output)
	if !strings.Contains(text, ":: "+version.Name+" ::") {
		t.Fatalf("banner does not contain canonical brand: %q", text)
	}
	if !strings.Contains(text, "("+version.Current+")") {
		t.Fatalf("banner does not contain canonical version: %q", text)
	}
	for _, legacy := range []string{"SpringGo", "Spring Go", "v4.0", "RELEASE"} {
		if strings.Contains(text, legacy) {
			t.Fatalf("banner contains legacy identity %q: %q", legacy, text)
		}
	}
}
