package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var (
	swaggerMainFile string
	swaggerQuiet    bool

	swaggerCmd = &cobra.Command{
		Use:   "swagger",
		Short: "Generate Swagger documentation",
		Long:  "Generates Swagger documentation explicitly, keeping the hot-reload build loop fast.",
		RunE: func(cmd *cobra.Command, args []string) error {
			swagPath, err := exec.LookPath("swag")
			if err != nil {
				return fmt.Errorf("swag is not installed; run: go install github.com/swaggo/swag/cmd/swag@latest")
			}

			fmt.Println("📝 Generating Swagger documentation...")
			swag := exec.Command(swagPath, swaggerArgs(swaggerMainFile, swaggerQuiet)...)
			swag.Stdout = os.Stdout
			swag.Stderr = os.Stderr
			if err := swag.Run(); err != nil {
				return fmt.Errorf("Swagger generation failed: %w", err)
			}
			fmt.Println("✅ Swagger documentation generated successfully")
			return nil
		},
	}
)

func swaggerArgs(mainFile string, quiet bool) []string {
	args := []string{
		"init",
		"-g", mainFile,
		"--parseInternal",
		"--pdl", "1",
		"--parseGoList=false",
	}
	if quiet {
		args = append(args, "-q")
	}
	return args
}

func init() {
	swaggerCmd.Flags().StringVarP(&swaggerMainFile, "main", "g", "cmd/app/main.go", "Go file containing the general API annotations")
	swaggerCmd.Flags().BoolVarP(&swaggerQuiet, "quiet", "q", false, "Suppress swag generator output")
	rootCmd.AddCommand(swaggerCmd)
}
