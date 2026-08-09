package cmd

import (
	"github.com/spf13/cobra"
)

var routesCmd = &cobra.Command{
	Use:   "routes",
	Short: "Display all registered REST routes in a formatted table",
	Long:  `Inspects and prints all HTTP endpoints registered in the SprinGo web router.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProjectAction("routes")
	},
}

func init() {
	rootCmd.AddCommand(routesCmd)
}
