package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/NeftaliAcosta/springo/framework/version"
	"github.com/spf13/cobra"
)

var (
	localFlag   bool
	skipGitFlag bool

	newCmd = &cobra.Command{
		Use:   "new <project-name>",
		Short: "Create a new SprinGo project with Hexagonal Architecture",
		Long: `Create a clean, production-ready SprinGo web application.
Generates directory structure, domain models, services, REST controllers, Flyway-style migrations, application.yaml, .gitignore and initializes git.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectName := args[0]
			frameworkModule := "github.com/NeftaliAcosta/springo"

			fmt.Printf("🌱 Generating new %s project (%s Hexagonal): %s...\n", version.Name, version.Current, projectName)
			if localFlag {
				fmt.Println("🔧 Development mode: Linking local framework module via replace directive")
			}

			pwd, _ := filepath.Abs(".")
			config := ProjectConfig{
				ProjectName:        projectName,
				FrameworkModule:    frameworkModule,
				IsLocal:            localFlag,
				LocalFrameworkPath: pwd,
				SkipGit:            skipGitFlag,
			}

			if err := GenerateProject(config); err != nil {
				return fmt.Errorf("failed to generate project: %w", err)
			}

			fmt.Printf("\n🎉 Project %s created successfully!\n", projectName)
			fmt.Println("\nNext steps:")
			fmt.Printf("  1. cd %s\n", projectName)
			fmt.Println("  2. go mod tidy")
			fmt.Println("  3. springo run (or go run cmd/app/main.go)")
			return nil
		},
	}
)

func init() {
	newCmd.Flags().BoolVarP(&localFlag, "local", "l", false, "Configure go.mod to use local framework directory")
	newCmd.Flags().BoolVar(&skipGitFlag, "skip-git", false, "Skip git repository initialization")
	rootCmd.AddCommand(newCmd)
}
