package cmd

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/spf13/cobra"
)

var makeCmd = &cobra.Command{
	Use:   "make",
	Short: "Generate code components (controllers, services, models, DTOs, migrations, jobs, events, config)",
	Long: `The make command family allows you to generate clean boilerplate components for your SprinGo application,
following the Hexagonal Architecture standards.`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

type MakeTemplateData struct {
	StructName      string
	LowerName       string
	PluralName      string
	PackageName     string
	ModulePath      string
	FrameworkModule string
	MigrationName   string
	ProjectName     string
}

func getModulePath() string {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return "springo"
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return "springo"
}

func getFrameworkModule() string {
	return "github.com/NeftaliAcosta/springo"
}

func renderMakeTemplate(tmplPath string, data MakeTemplateData) ([]byte, error) {
	tmplBytes, err := TemplatesFS.ReadFile(tmplPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded template %s: %w", tmplPath, err)
	}
	tmpl, err := template.New(tmplPath).Parse(string(tmplBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	raw := buf.Bytes()
	// Format Go files automatically using go/format
	if strings.HasSuffix(tmplPath, ".tmpl") && !strings.Contains(tmplPath, "config.tmpl") {
		formatted, err := format.Source(raw)
		if err == nil {
			return formatted, nil
		}
	}
	return raw, nil
}

func writeGeneratedFile(targetPath string, content []byte) error {
	if _, err := os.Stat(targetPath); err == nil {
		return fmt.Errorf("file '%s' already exists", targetPath)
	}
	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	return os.WriteFile(targetPath, content, 0644)
}

var validIdentifierRegex = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

func isValidIdentifier(name string) bool {
	return validIdentifierRegex.MatchString(name)
}

func buildTemplateData(rawName string) (MakeTemplateData, error) {
	clean := strings.TrimSpace(rawName)
	if !isValidIdentifier(clean) {
		return MakeTemplateData{}, fmt.Errorf("invalid name '%s': must be a valid Go identifier starting with a letter", rawName)
	}

	// Capitalize first letter safely
	name := strings.ToUpper(clean[:1]) + clean[1:]
	lower := strings.ToLower(name)
	plural := lower + "s"
	return MakeTemplateData{
		StructName:      name,
		LowerName:       lower,
		PluralName:      plural,
		ModulePath:      getModulePath(),
		FrameworkModule: getFrameworkModule(),
	}, nil
}

func init() {
	makeCmd.AddCommand(makeControllerCmd)
	makeCmd.AddCommand(makeServiceCmd)
	makeCmd.AddCommand(makeRepoCmd)
	makeCmd.AddCommand(makeModelCmd)
	makeCmd.AddCommand(makeDtoCmd)
	makeCmd.AddCommand(makeMigrationCmd)
	makeCmd.AddCommand(makeJobCmd)
	makeCmd.AddCommand(makeEventCmd)
	makeCmd.AddCommand(makeConfigCmd)
}

var makeControllerCmd = &cobra.Command{
	Use:   "controller <Name>",
	Short: "Generate a REST controller",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := buildTemplateData(args[0])
		if err != nil {
			return err
		}
		data.PackageName = "rest"

		content, err := renderMakeTemplate("templates/make/controller.tmpl", data)
		if err != nil {
			return err
		}
		target := filepath.Join("internal", "infrastructure", "input", "rest", data.LowerName+"_controller.go")
		if err := writeGeneratedFile(target, content); err != nil {
			return err
		}
		fmt.Printf("✅ Created REST controller: %s\n", target)
		return nil
	},
}

var makeServiceCmd = &cobra.Command{
	Use:   "service <Name>",
	Short: "Generate an application service and use case interface",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := buildTemplateData(args[0])
		if err != nil {
			return err
		}

		contentUseCase, err := renderMakeTemplate("templates/make/service_interface.tmpl", data)
		if err != nil {
			return fmt.Errorf("failed to render use case template: %w", err)
		}
		targetUseCase := filepath.Join("internal", "domain", "port", "in", data.LowerName+"_use_case.go")
		if err := writeGeneratedFile(targetUseCase, contentUseCase); err != nil {
			return err
		}
		fmt.Printf("✅ Created use case interface: %s\n", targetUseCase)

		contentServiceImpl, err := renderMakeTemplate("templates/make/service_impl.tmpl", data)
		if err != nil {
			return fmt.Errorf("failed to render service impl template: %w", err)
		}
		targetServiceImpl := filepath.Join("internal", "application", "service", data.LowerName+"_service.go")
		if err := writeGeneratedFile(targetServiceImpl, contentServiceImpl); err != nil {
			return err
		}
		fmt.Printf("✅ Created application service: %s\n", targetServiceImpl)
		return nil
	},
}

var makeRepoCmd = &cobra.Command{
	Use:   "repository <Name>",
	Short: "Generate a persistence port, GORM entity and repository adapter",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := buildTemplateData(args[0])
		if err != nil {
			return err
		}

		// 1. Port Interface
		contentPort, err := renderMakeTemplate("templates/make/repo_interface.tmpl", data)
		if err != nil {
			return fmt.Errorf("failed to render repo interface: %w", err)
		}
		targetPort := filepath.Join("internal", "application", "port", "output", data.LowerName+"_persistence_port.go")
		if err := writeGeneratedFile(targetPort, contentPort); err != nil {
			return err
		}
		fmt.Printf("✅ Created persistence port: %s\n", targetPort)

		// 2. GORM Entity
		contentEntity, err := renderMakeTemplate("templates/make/repo_entity.tmpl", data)
		if err != nil {
			return fmt.Errorf("failed to render repo entity: %w", err)
		}
		targetEntity := filepath.Join("internal", "infrastructure", "output", "persistence", data.LowerName+"_entity.go")
		if err := writeGeneratedFile(targetEntity, contentEntity); err != nil {
			return err
		}
		fmt.Printf("✅ Created GORM entity: %s\n", targetEntity)

		// 3. Repository Adapter
		contentAdapter, err := renderMakeTemplate("templates/make/repo_impl.tmpl", data)
		if err != nil {
			return fmt.Errorf("failed to render repo adapter: %w", err)
		}
		targetAdapter := filepath.Join("internal", "infrastructure", "output", "persistence", data.LowerName+"_repository_adapter.go")
		if err := writeGeneratedFile(targetAdapter, contentAdapter); err != nil {
			return err
		}
		fmt.Printf("✅ Created repository adapter: %s\n", targetAdapter)
		return nil
	},
}

var makeModelCmd = &cobra.Command{
	Use:   "model <Name>",
	Short: "Generate a domain model",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := buildTemplateData(args[0])
		if err != nil {
			return err
		}
		content, err := renderMakeTemplate("templates/make/model.tmpl", data)
		if err != nil {
			return err
		}
		target := filepath.Join("internal", "domain", "model", data.LowerName+".go")
		if err := writeGeneratedFile(target, content); err != nil {
			return err
		}
		fmt.Printf("✅ Created domain model: %s\n", target)
		return nil
	},
}

var makeDtoCmd = &cobra.Command{
	Use:   "dto <Name>",
	Short: "Generate Request and Response DTOs",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := buildTemplateData(args[0])
		if err != nil {
			return err
		}

		contentReq, err := renderMakeTemplate("templates/make/dto_request.tmpl", data)
		if err != nil {
			return fmt.Errorf("failed to render DTO request: %w", err)
		}
		targetReq := filepath.Join("internal", "infrastructure", "dtos", "request", data.LowerName+"_request.go")
		if err := writeGeneratedFile(targetReq, contentReq); err != nil {
			return err
		}
		fmt.Printf("✅ Created Request DTO: %s\n", targetReq)

		contentRes, err := renderMakeTemplate("templates/make/dto_response.tmpl", data)
		if err != nil {
			return fmt.Errorf("failed to render DTO response: %w", err)
		}
		targetRes := filepath.Join("internal", "infrastructure", "dtos", "response", data.LowerName+"_response.go")
		if err := writeGeneratedFile(targetRes, contentRes); err != nil {
			return err
		}
		fmt.Printf("✅ Created Response DTO: %s\n", targetRes)
		return nil
	},
}

var makeMigrationCmd = &cobra.Command{
	Use:   "migration <Name>",
	Short: "Generate a Flyway-style migration file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		timestamp := time.Now().Format("20060102_150405")
		migName := timestamp + "_" + strings.ToLower(args[0])
		data := MakeTemplateData{
			MigrationName:   migName,
			FrameworkModule: getFrameworkModule(),
		}
		content, err := renderMakeTemplate("templates/make/migration.tmpl", data)
		if err != nil {
			return err
		}
		target := filepath.Join("resources", "db", "migration", migName+".go")
		if err := writeGeneratedFile(target, content); err != nil {
			return err
		}
		fmt.Printf("✅ Created migration file: %s\n", target)
		return nil
	},
}

var makeJobCmd = &cobra.Command{
	Use:   "job <Name>",
	Short: "Generate a scheduled job task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := buildTemplateData(args[0])
		if err != nil {
			return err
		}
		content, err := renderMakeTemplate("templates/make/job.tmpl", data)
		if err != nil {
			return err
		}
		target := filepath.Join("internal", "infrastructure", "input", "scheduler", data.LowerName+"_job.go")
		if err := writeGeneratedFile(target, content); err != nil {
			return err
		}
		fmt.Printf("✅ Created scheduled job: %s\n", target)
		return nil
	},
}

var makeEventCmd = &cobra.Command{
	Use:   "event <Name>",
	Short: "Generate a domain event and event listener",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := buildTemplateData(args[0])
		if err != nil {
			return err
		}
		content, err := renderMakeTemplate("templates/make/event.tmpl", data)
		if err != nil {
			return err
		}
		target := filepath.Join("internal", "infrastructure", "input", "events", data.LowerName+"_event.go")
		if err := writeGeneratedFile(target, content); err != nil {
			return err
		}
		fmt.Printf("✅ Created domain event & listener: %s\n", target)
		return nil
	},
}

var makeConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Generate a full application.yaml configuration file",
	RunE: func(cmd *cobra.Command, args []string) error {
		data := MakeTemplateData{ProjectName: "springo-app"}
		content, err := renderMakeTemplate("templates/make/config.tmpl", data)
		if err != nil {
			return err
		}
		target := filepath.Join("resources", "application.yaml")
		if err := writeGeneratedFile(target, content); err != nil {
			return err
		}
		fmt.Printf("✅ Created configuration file: %s\n", target)
		return nil
	},
}
