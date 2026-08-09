package cmd

import (
	"fmt"
	"os"

	"github.com/NeftaliAcosta/springo/framework/version"
	"github.com/spf13/cobra"
)

var (
	// Version holds the current CLI version
	Version = version.Current

	rootCmd = &cobra.Command{
		Use:   "springo",
		Short: "SprinGo CLI — Enterprise Web Framework for Go",
		Long: `SprinGo CLI is the official command-line tool for the SprinGo framework.
It provides generators, migration tools, scaffolding and development utilities.`,
	}
)

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(makeCmd)
	rootCmd.AddCommand(migrateCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of SprinGo CLI",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("%s CLI %s (Framework Core %s)\n", version.Name, Version, version.Current)
	},
}
