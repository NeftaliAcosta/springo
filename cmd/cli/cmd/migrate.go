package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var stepsFlag int

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Execute pending database migrations",
	Long:  `Executes all pending Flyway-style database migrations with cluster-safe locking.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProjectAction("migrate")
	},
}

var migrateRollbackCmd = &cobra.Command{
	Use:   "rollback",
	Short: "Rollback database migrations",
	Long:  `Rollback the last batch of migrations or specified N steps (--steps=N).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProjectAction("rollback", fmt.Sprintf("%d", stepsFlag))
	},
}

var migrateResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Rollback all executed database migrations",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProjectAction("reset")
	},
}

var migrateRefreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Reset and re-run all database migrations from scratch",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProjectAction("refresh")
	},
}

var migrateStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the status table of all registered migrations",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProjectAction("status")
	},
}

func init() {
	migrateRollbackCmd.Flags().IntVarP(&stepsFlag, "steps", "s", 0, "Number of migration steps to rollback")

	migrateCmd.AddCommand(migrateRollbackCmd)
	migrateCmd.AddCommand(migrateResetCmd)
	migrateCmd.AddCommand(migrateRefreshCmd)
	migrateCmd.AddCommand(migrateStatusCmd)
}
