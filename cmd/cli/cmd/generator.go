package cmd

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

//go:embed templates/*
var TemplatesFS embed.FS

type ProjectConfig struct {
	ProjectName        string
	ModuleName         string
	FrameworkModule    string
	IsLocal            bool
	LocalFrameworkPath string
	SkipGit            bool
}

var subdirs = []string{
	"cmd/app",
	"internal/domain/model",
	"internal/domain/errors",
	"internal/domain/event",
	"internal/domain/port/in",
	"internal/application/port/output",
	"internal/application/service",
	"internal/infrastructure/config",
	"internal/infrastructure/dtos/request",
	"internal/infrastructure/dtos/response",
	"internal/infrastructure/mappers",
	"internal/infrastructure/input/rest",
	"internal/infrastructure/input/events",
	"internal/infrastructure/input/scheduler",
	"internal/infrastructure/output/persistence",
	"internal/infrastructure/security",
	"resources/db/migration",
}

func GenerateProject(config ProjectConfig) error {
	if config.ModuleName == "" {
		config.ModuleName = filepath.Base(config.ProjectName)
	}
	if config.FrameworkModule == "" {
		config.FrameworkModule = "github.com/NeftaliAcosta/springo"
	}

	if _, err := os.Stat(config.ProjectName); !os.IsNotExist(err) {
		return fmt.Errorf("directory '%s' already exists", config.ProjectName)
	}

	for _, dir := range subdirs {
		path := filepath.Join(config.ProjectName, dir)
		if err := os.MkdirAll(path, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", path, err)
		}
	}

	fileMap := map[string]string{
		"templates/air_toml.tmpl":                ".air.toml",
		"templates/go_mod.tmpl":                  "go.mod",
		"templates/main_go.tmpl":                 "cmd/app/main.go",
		"templates/application_yaml.tmpl":        "resources/application.yaml",
		"templates/user_model.tmpl":              "internal/domain/model/user.go",
		"templates/errors.tmpl":                  "internal/domain/errors/errors.go",
		"templates/user_use_case.tmpl":           "internal/domain/port/in/user_use_case.go",
		"templates/user_persistence_port.tmpl":   "internal/application/port/output/user_persistence_port.go",
		"templates/user_service.tmpl":            "internal/application/service/user_service.go",
		"templates/user_entity.tmpl":             "internal/infrastructure/output/persistence/user_entity.go",
		"templates/user_repository_adapter.tmpl": "internal/infrastructure/output/persistence/user_repository_adapter.go",
		"templates/user_controller.tmpl":         "internal/infrastructure/input/rest/user_controller.go",
		"templates/migration.tmpl":               "resources/db/migration/20260614_000001_create_users_table.go",
	}

	for tmplPath, targetRelPath := range fileMap {
		content, err := renderTemplate(tmplPath, config)
		if err != nil {
			return fmt.Errorf("failed to render template %s: %w", tmplPath, err)
		}

		targetFullPath := filepath.Join(config.ProjectName, targetRelPath)
		if err := os.WriteFile(targetFullPath, content, 0644); err != nil {
			return fmt.Errorf("failed to write file %s: %w", targetFullPath, err)
		}
	}

	// Create default .gitignore
	gitignoreContent := []byte("*.db\n*.db-shm\n*.db-wal\n/tmp/\n/bin/\n.DS_Store\n")
	_ = os.WriteFile(filepath.Join(config.ProjectName, ".gitignore"), gitignoreContent, 0644)

	// Post-generation tasks
	if !config.SkipGit {
		initGitRepo(config.ProjectName)
	}

	return nil
}

func initGitRepo(projectPath string) {
	cmd := exec.Command("git", "init")
	cmd.Dir = projectPath
	_ = cmd.Run()
}

func renderTemplate(tmplPath string, config ProjectConfig) ([]byte, error) {
	tmplBytes, err := TemplatesFS.ReadFile(tmplPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded template file %s: %w", tmplPath, err)
	}

	tmpl, err := template.New(tmplPath).Parse(string(tmplBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, config); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.Bytes(), nil
}
