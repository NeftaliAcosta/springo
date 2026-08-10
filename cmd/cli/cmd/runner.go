package cmd

import (
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
)

// runProjectAction creates a unique temporary directory, compiles and executes a project action, and cleans up.
func runProjectAction(action string, extraArgs ...string) error {
	modulePath := getModulePath()
	frameworkModule := getFrameworkModule()

	tmpDir := filepath.Join(".springo", "tmp")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}

	var rawCode string
	switch action {
	case "migrate":
		rawCode = fmt.Sprintf(`package main

import (
	"fmt"
	"os"
	"%s/framework/config"
	"%s/framework/database"
	_ "%s/resources/db/migration"
)

func main() {
	loader := config.NewConfigLoader()
	_ = loader.Load()
	_ = config.InitializeProperties(loader)

	dbProps := config.Get[database.DataSourceProperties]()
	if dbProps == nil || dbProps.Url == "" {
		dbProps = &database.DataSourceProperties{Driver: "sqlite", Url: "springo.db"}
	}
	db, err := database.Connect(dbProps)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Connection error: %%v\n", err)
		os.Exit(1)
	}
	sqlDB, errDB := db.DB()
	if errDB == nil && sqlDB != nil {
		defer sqlDB.Close()
	}

	mgr := database.NewMigrationManager(db, false)
	if err := mgr.Migrate(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Migration error: %%v\n", err)
		os.Exit(1)
	}
}
`, frameworkModule, frameworkModule, modulePath)

	case "rollback":
		steps := 0
		if len(extraArgs) > 0 {
			_, _ = fmt.Sscanf(extraArgs[0], "%d", &steps)
		}
		rawCode = fmt.Sprintf(`package main

import (
	"fmt"
	"os"
	"%s/framework/config"
	"%s/framework/database"
	_ "%s/resources/db/migration"
)

func main() {
	loader := config.NewConfigLoader()
	_ = loader.Load()
	_ = config.InitializeProperties(loader)

	dbProps := config.Get[database.DataSourceProperties]()
	if dbProps == nil || dbProps.Url == "" {
		dbProps = &database.DataSourceProperties{Driver: "sqlite", Url: "springo.db"}
	}
	db, err := database.Connect(dbProps)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Connection error: %%v\n", err)
		os.Exit(1)
	}
	sqlDB, errDB := db.DB()
	if errDB == nil && sqlDB != nil {
		defer sqlDB.Close()
	}

	mgr := database.NewMigrationManager(db, false)
	if err := mgr.Rollback(%d); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Rollback error: %%v\n", err)
		os.Exit(1)
	}
}
`, frameworkModule, frameworkModule, modulePath, steps)

	case "reset":
		rawCode = fmt.Sprintf(`package main

import (
	"fmt"
	"os"
	"%s/framework/config"
	"%s/framework/database"
	_ "%s/resources/db/migration"
)

func main() {
	loader := config.NewConfigLoader()
	_ = loader.Load()
	_ = config.InitializeProperties(loader)

	dbProps := config.Get[database.DataSourceProperties]()
	if dbProps == nil || dbProps.Url == "" {
		dbProps = &database.DataSourceProperties{Driver: "sqlite", Url: "springo.db"}
	}
	db, err := database.Connect(dbProps)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Connection error: %%v\n", err)
		os.Exit(1)
	}
	sqlDB, errDB := db.DB()
	if errDB == nil && sqlDB != nil {
		defer sqlDB.Close()
	}

	mgr := database.NewMigrationManager(db, false)
	if err := mgr.Reset(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Reset error: %%v\n", err)
		os.Exit(1)
	}
}
`, frameworkModule, frameworkModule, modulePath)

	case "refresh":
		rawCode = fmt.Sprintf(`package main

import (
	"fmt"
	"os"
	"%s/framework/config"
	"%s/framework/database"
	_ "%s/resources/db/migration"
)

func main() {
	loader := config.NewConfigLoader()
	_ = loader.Load()
	_ = config.InitializeProperties(loader)

	dbProps := config.Get[database.DataSourceProperties]()
	if dbProps == nil || dbProps.Url == "" {
		dbProps = &database.DataSourceProperties{Driver: "sqlite", Url: "springo.db"}
	}
	db, err := database.Connect(dbProps)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Connection error: %%v\n", err)
		os.Exit(1)
	}
	sqlDB, errDB := db.DB()
	if errDB == nil && sqlDB != nil {
		defer sqlDB.Close()
	}

	mgr := database.NewMigrationManager(db, false)
	if err := mgr.Refresh(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Refresh error: %%v\n", err)
		os.Exit(1)
	}
}
`, frameworkModule, frameworkModule, modulePath)

	case "status":
		rawCode = fmt.Sprintf(`package main

import (
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"
	"time"
	"%s/framework/config"
	"%s/framework/database"
	_ "%s/resources/db/migration"
)

func main() {
	loader := config.NewConfigLoader()
	_ = loader.Load()
	_ = config.InitializeProperties(loader)

	dbProps := config.Get[database.DataSourceProperties]()
	if dbProps == nil || dbProps.Url == "" {
		dbProps = &database.DataSourceProperties{Driver: "sqlite", Url: "springo.db"}
	}
	db, err := database.Connect(dbProps)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Connection error: %%v\n", err)
		os.Exit(1)
	}
	sqlDB, errDB := db.DB()
	if errDB == nil && sqlDB != nil {
		defer sqlDB.Close()
	}

	mgr := database.NewMigrationManager(db, false)
	statusList, err := mgr.GetStatus()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Status error: %%v\n", err)
		os.Exit(1)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "STATUS\tBATCH\tMIGRATION\tAPPLIED AT")
	fmt.Fprintln(w, "------\t-----\t---------\t----------")
	for _, s := range statusList {
		st := "PENDING"
		batchStr := "-"
		appliedStr := "-"
		if s.Executed {
			st = "RAN"
			batchStr = strconv.Itoa(s.Batch)
			appliedStr = s.AppliedAt.Format(time.RFC3339)
		}
		fmt.Fprintf(w, "%%s\t%%s\t%%s\t%%s\n", st, batchStr, s.Name, appliedStr)
	}
	w.Flush()
}
`, frameworkModule, frameworkModule, modulePath)

	case "routes":
		rawCode = fmt.Sprintf(`package main

import (
	"fmt"
	"os"
	"text/tabwriter"
	"%s/framework/config"
	"%s/framework/web"
	_ "%s/internal/infrastructure/input/rest"
)

func main() {
	loader := config.NewConfigLoader().WithBaseDir("resources")
	if err := loader.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to load application configuration: %%v\n", err)
		os.Exit(1)
	}
	if err := config.InitializeProperties(loader); err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize application properties: %%v\n", err)
		os.Exit(1)
	}
	routes := web.GetRegisteredRoutes()
	if len(routes) == 0 {
		fmt.Println("ℹ️ No registered routes found.")
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "METHOD\tPATTERN")
	fmt.Fprintln(w, "------\t-------")
	for _, r := range routes {
		fmt.Fprintf(w, "%%s\t%%s\n", r.Method, r.Pattern)
	}
	w.Flush()
}
`, frameworkModule, frameworkModule, modulePath)
	}

	formattedCode, err := format.Source([]byte(rawCode))
	if err != nil {
		return fmt.Errorf("failed to format generated runner for action %q: %w", action, err)
	}

	tmp, err := os.CreateTemp(tmpDir, "runner_*.go")
	if err != nil {
		return fmt.Errorf("failed to create temporary runner: %w", err)
	}
	tmpFile := tmp.Name()
	defer os.Remove(tmpFile)

	if _, err := tmp.Write(formattedCode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to write runner code: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close runner code: %w", err)
	}

	cmd := exec.Command("go", "run", tmpFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
