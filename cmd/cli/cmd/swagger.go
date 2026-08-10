package cmd

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

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
	generalInfo, searchDirs := swaggerSearchConfig(mainFile)
	args := []string{
		"init",
		"-g", generalInfo,
		"-d", searchDirs,
		"--parseInternal",
		"--pdl", "1",
		"--parseGoList=false",
	}
	if quiet {
		args = append(args, "-q")
	}
	return args
}

func swaggerSearchConfig(mainFile string) (generalInfo, searchDirs string) {
	cleanMainFile := filepath.Clean(mainFile)
	mainDir := filepath.Dir(cleanMainFile)
	generalInfo = filepath.Base(cleanMainFile)
	if mainDir == "." {
		return generalInfo, "."
	}
	dirs := []string{mainDir}
	dirs = append(dirs, collectGoPackageDirs("internal")...)
	return generalInfo, strings.Join(dirs, ",")
}

func collectGoPackageDirs(root string) []string {
	dirs := map[string]struct{}{}
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		dirs[filepath.Dir(path)] = struct{}{}
		return nil
	})
	result := make([]string, 0, len(dirs))
	for dir := range dirs {
		result = append(result, dir)
	}
	sort.Strings(result)
	return result
}

func init() {
	swaggerCmd.Flags().StringVarP(&swaggerMainFile, "main", "g", "cmd/app/main.go", "Go file containing the general API annotations")
	swaggerCmd.Flags().BoolVarP(&swaggerQuiet, "quiet", "q", false, "Suppress swag generator output")
	rootCmd.AddCommand(swaggerCmd)
}
