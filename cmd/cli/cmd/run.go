package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"

	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run application in development mode with Hot Reload",
	Long:  `Executes the SprinGo web application and automatically recompiles on code changes.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("🚀 Starting SprinGo application in development mode...")

		// Check if air is installed for hot reload
		airPath, err := exec.LookPath("air")
		if err == nil {
			fmt.Println("🔥 Hot Reload enabled (using Air)...")
			airCmd := exec.Command(airPath)
			airCmd.Stdout = os.Stdout
			airCmd.Stderr = os.Stderr
			return airCmd.Run()
		}

		// Fallback to standard go run
		fmt.Println("ℹ️ Air not found. Running with standard 'go run cmd/app/main.go'...")
		fmt.Println("💡 Tip: Install Air (go install github.com/air-verse/air@latest) for automatic hot reload.")

		goCmd := exec.Command("go", "run", "cmd/app/main.go")
		goCmd.Stdout = os.Stdout
		goCmd.Stderr = os.Stderr

		// Handle interrupt signals
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt)

		if err := goCmd.Start(); err != nil {
			return fmt.Errorf("failed to start application: %w", err)
		}

		done := make(chan error, 1)
		go func() {
			done <- goCmd.Wait()
		}()

		select {
		case err := <-done:
			return err
		case <-sigChan:
			fmt.Println("\n🛑 Shutting down SprinGo application...")
			if goCmd.Process != nil {
				_ = goCmd.Process.Kill()
			}
			return nil
		}
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
}
