package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

func TestMakeControllerValidation(t *testing.T) {
	err := makeControllerCmd.RunE(makeControllerCmd, []string{"invalid-name!"})
	if err == nil {
		t.Fatalf("expected error for invalid identifier, got nil")
	}
}

func TestMakeModelValidation(t *testing.T) {
	err := makeModelCmd.RunE(makeModelCmd, []string{"123bad"})
	if err == nil {
		t.Fatalf("expected error for identifier starting with digits, got nil")
	}
}

func TestMakeScaffoldingInTempDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "springo_cli_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	config := ProjectConfig{
		ProjectName:        filepath.Join(tmpDir, "test_proj"),
		ModuleName:         "test_proj",
		FrameworkModule:    "github.com/NeftaliAcosta/springo",
		IsLocal:            true,
		LocalFrameworkPath: ".",
		SkipGit:            true,
	}

	if err := GenerateProject(config); err != nil {
		t.Fatalf("failed to generate project: %v", err)
	}

	// Verify essential files exist
	expectedFiles := []string{
		".air.toml",
		"go.mod",
		"cmd/app/main.go",
		"resources/application.yaml",
		".gitignore",
		"internal/domain/model/user.go",
		"resources/db/migration/20260614_000001_create_users_table.go",
	}

	for _, file := range expectedFiles {
		fullPath := filepath.Join(config.ProjectName, file)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			t.Errorf("expected generated file %s to exist", file)
		}
	}
}

func TestFullEndToEndCLIWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E CLI workflow test in short mode")
	}

	tmpDir, err := os.MkdirTemp("", "springo_e2e_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	absRoot, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("failed to resolve root path: %v", err)
	}

	projDir := filepath.Join(tmpDir, "test-app")
	config := ProjectConfig{
		ProjectName:        projDir,
		ModuleName:         "test-app",
		FrameworkModule:    "github.com/NeftaliAcosta/springo",
		IsLocal:            true,
		LocalFrameworkPath: absRoot,
		SkipGit:            true,
	}

	if err := GenerateProject(config); err != nil {
		t.Fatalf("GenerateProject failed: %v", err)
	}

	// Change working directory to generated project for make commands & runner testing
	origWd, _ := os.Getwd()
	if err := os.Chdir(projDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	// Execute all make generators
	components := []string{"Widget"}
	for _, comp := range components {
		if err := makeModelCmd.RunE(makeModelCmd, []string{comp}); err != nil {
			t.Fatalf("make model failed: %v", err)
		}
		if err := makeDtoCmd.RunE(makeDtoCmd, []string{comp}); err != nil {
			t.Fatalf("make dto failed: %v", err)
		}
		if err := makeRepoCmd.RunE(makeRepoCmd, []string{comp}); err != nil {
			t.Fatalf("make repo failed: %v", err)
		}
		if err := makeServiceCmd.RunE(makeServiceCmd, []string{comp}); err != nil {
			t.Fatalf("make service failed: %v", err)
		}
		if err := makeControllerCmd.RunE(makeControllerCmd, []string{comp}); err != nil {
			t.Fatalf("make controller failed: %v", err)
		}
		if err := makeMigrationCmd.RunE(makeMigrationCmd, []string{"CreateWidgetsTable"}); err != nil {
			t.Fatalf("make migration failed: %v", err)
		}
	}

	// Run go mod tidy
	cmdTidy := exec.Command("go", "mod", "tidy")
	if out, err := cmdTidy.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy failed: %v\nOutput: %s", err, string(out))
	}

	// Verify project compiles completely with go test ./...
	cmdTest := exec.Command("go", "test", "./...")
	if out, err := cmdTest.CombinedOutput(); err != nil {
		t.Fatalf("go test ./... failed in generated project: %v\nOutput: %s", err, string(out))
	}

	// Test migration runner status and execution
	if err := runProjectAction("migrate"); err != nil {
		t.Fatalf("runProjectAction migrate failed: %v", err)
	}

	if err := runProjectAction("status"); err != nil {
		t.Fatalf("runProjectAction status failed: %v", err)
	}

	if err := runProjectAction("routes"); err != nil {
		t.Fatalf("runProjectAction routes failed: %v", err)
	}

	// Concurrent invocations must use distinct runner files and clean up only
	// their own files.
	const concurrentRunners = 4
	var wg sync.WaitGroup
	errCh := make(chan error, concurrentRunners)
	for i := 0; i < concurrentRunners; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- runProjectAction("routes")
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent routes runner failed: %v", err)
		}
	}

	remaining, err := filepath.Glob(filepath.Join(".springo", "tmp", "runner_*.go"))
	if err != nil {
		t.Fatalf("failed to inspect runner cleanup: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("temporary runners were not cleaned up: %v", remaining)
	}

	if err := runProjectAction("rollback", "1"); err != nil {
		t.Fatalf("runProjectAction rollback failed: %v", err)
	}

	if err := runProjectAction("refresh"); err != nil {
		t.Fatalf("runProjectAction refresh failed: %v", err)
	}

	if err := runProjectAction("reset"); err != nil {
		t.Fatalf("runProjectAction reset failed: %v", err)
	}
}
