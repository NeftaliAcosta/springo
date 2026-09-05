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
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

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
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

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

	executeMakeGenerators(t, "Widget")
	verifyProjectBuild(t)

	verifyProjectActions(t, []string{"migrate"}, []string{"status"}, []string{"routes"})
	verifyConcurrentRunners(t)
	verifyProjectActions(t, []string{"rollback", "1"}, []string{"refresh"}, []string{"reset"})
}

func executeMakeGenerators(t *testing.T, component string) {
	t.Helper()
	commands := []struct {
		name string
		run  func() error
	}{
		{"model", func() error { return makeModelCmd.RunE(makeModelCmd, []string{component}) }},
		{"dto", func() error { return makeDtoCmd.RunE(makeDtoCmd, []string{component}) }},
		{"repo", func() error { return makeRepoCmd.RunE(makeRepoCmd, []string{component}) }},
		{"service", func() error { return makeServiceCmd.RunE(makeServiceCmd, []string{component}) }},
		{"controller", func() error { return makeControllerCmd.RunE(makeControllerCmd, []string{component}) }},
		{"migration", func() error { return makeMigrationCmd.RunE(makeMigrationCmd, []string{"CreateWidgetsTable"}) }},
	}

	for _, cmd := range commands {
		if err := cmd.run(); err != nil {
			t.Fatalf("make %s failed: %v", cmd.name, err)
		}
	}

	sqlMigrationFlag = true
	defer func() { sqlMigrationFlag = false }()
	if err := makeMigrationCmd.RunE(makeMigrationCmd, []string{"create_orders_table"}); err != nil {
		t.Fatalf("make sql migration failed: %v", err)
	}
}

func verifyProjectBuild(t *testing.T) {
	t.Helper()
	cmdTidy := exec.Command("go", "mod", "tidy")
	if out, err := cmdTidy.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy failed: %v\nOutput: %s", err, string(out))
	}

	cmdTest := exec.Command("go", "test", "./...")
	if out, err := cmdTest.CombinedOutput(); err != nil {
		t.Fatalf("go test ./... failed in generated project: %v\nOutput: %s", err, string(out))
	}
}

func verifyConcurrentRunners(t *testing.T) {
	t.Helper()
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
}

func verifyProjectActions(t *testing.T, actions ...[]string) {
	t.Helper()
	for _, action := range actions {
		if len(action) == 0 {
			continue
		}
		if err := runProjectAction(action[0], action[1:]...); err != nil {
			t.Fatalf("runProjectAction %v failed: %v", action, err)
		}
	}
}

