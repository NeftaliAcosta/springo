package database

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/NeftaliAcosta/springo/framework/config"
	"gorm.io/gorm"
)

// DiscoverSQLMigrations scans specified directory locations for versioned SQL migration files.
func DiscoverSQLMigrations(locations []string, prefix string) ([]Migration, error) {
	effectiveLocations := resolveLocations(locations)
	effectivePrefix := resolvePrefix(prefix)

	undoMap := make(map[string]string)
	var forwardFiles []string

	for _, loc := range effectiveLocations {
		if err := scanLocation(loc, &forwardFiles, undoMap); err != nil {
			return nil, err
		}
	}

	migrations, err := buildMigrations(forwardFiles, undoMap, effectivePrefix)
	if err != nil {
		return nil, err
	}

	sort.Slice(migrations, func(i, j int) bool {
		return compareMigrationNames(migrations[i].Name, migrations[j].Name)
	})

	return migrations, nil
}

func resolveLocations(locations []string) []string {
	if len(locations) == 0 {
		return []string{"resources/db/migration"}
	}
	return locations
}

func resolvePrefix(prefix string) string {
	if prefix == "" {
		return "V"
	}
	return prefix
}

func scanLocation(loc string, forwardFiles *[]string, undoMap map[string]string) error {
	entries, err := os.ReadDir(loc)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("scanning migration location %s: %w", loc, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		filePath := filepath.Join(loc, entry.Name())
		categorizeSQLFile(entry.Name(), filePath, forwardFiles, undoMap)
	}
	return nil
}

func categorizeSQLFile(name, filePath string, forwardFiles *[]string, undoMap map[string]string) {
	if isUndoFile(name) {
		baseKey := extractUndoBaseKey(name)
		undoMap[baseKey] = filePath
		return
	}
	*forwardFiles = append(*forwardFiles, filePath)
}

func isUndoFile(name string) bool {
	return strings.Contains(name, ".undo.") || strings.HasPrefix(name, "U")
}

func extractUndoBaseKey(name string) string {
	if strings.Contains(name, ".undo.sql") {
		return strings.Replace(name, ".undo.sql", ".sql", 1)
	}
	if strings.HasPrefix(name, "U") {
		return "V" + name[1:]
	}
	return name
}

func buildMigrations(files []string, undoMap map[string]string, prefix string) ([]Migration, error) {
	var migrations []Migration

	for _, file := range files {
		m, err := createSQLMigration(file, undoMap, prefix)
		if err != nil {
			return nil, err
		}
		migrations = append(migrations, m)
	}
	return migrations, nil
}

func createSQLMigration(file string, undoMap map[string]string, prefix string) (Migration, error) {
	content, err := os.ReadFile(file)
	if err != nil {
		return Migration{}, fmt.Errorf("reading SQL migration file %s: %w", file, err)
	}

	checksum := ComputeSQLChecksum(content)
	fileName := filepath.Base(file)
	migrationName := strings.TrimSuffix(fileName, ".sql")

	undoPath := findMatchingUndoPath(fileName, undoMap)

	return Migration{
		Name:     migrationName,
		Checksum: checksum,
		Up: func(db *gorm.DB) error {
			currentBytes, readErr := os.ReadFile(file)
			if readErr != nil {
				return fmt.Errorf("reading SQL migration file %s: %w", file, readErr)
			}
			if currentSum := ComputeSQLChecksum(currentBytes); currentSum != checksum {
				return fmt.Errorf("SQL migration %s was modified after discovery (expected %s, got %s)",
					file, checksum, currentSum)
			}
			return ExecuteSQLFile(db, file)
		},
		Down: buildDownFunc(undoPath),
	}, nil
}

func findMatchingUndoPath(fileName string, undoMap map[string]string) string {
	if path, ok := undoMap[fileName]; ok {
		return path
	}
	return ""
}

func buildDownFunc(undoPath string) func(db *gorm.DB) error {
	if undoPath == "" {
		return nil
	}
	return func(db *gorm.DB) error {
		return ExecuteSQLFile(db, undoPath)
	}
}

// ComputeSQLChecksum calculates a normalized SHA-256 hash for SQL file contents.
func ComputeSQLChecksum(content []byte) string {
	normalized := strings.ReplaceAll(string(content), "\r\n", "\n")
	normalized = strings.TrimSpace(normalized)
	hash := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(hash[:])
}

// AutoDiscoverAndRegisterSQLMigrations scans configured locations and registers discovered migrations.
func AutoDiscoverAndRegisterSQLMigrations(props *DataSourceProperties) error {
	if props == nil {
		props = config.Get[DataSourceProperties]()
	}

	locations := []string{"resources/db/migration"}
	prefix := "V"

	if props != nil {
		if len(props.Migration.Locations) > 0 {
			locations = props.Migration.Locations
		}
		prefix = props.GetSQLPrefix()
	}

	discovered, err := DiscoverSQLMigrations(locations, prefix)
	if err != nil {
		return err
	}

	registerDiscoveredMigrations(discovered)
	return nil
}

func registerDiscoveredMigrations(discovered []Migration) {
	existingMap := make(map[string]bool)
	for _, m := range getRegisteredMigrationsSnapshot() {
		existingMap[m.Name] = true
	}

	for _, m := range discovered {
		if !existingMap[m.Name] {
			RegisterMigration(m)
			existingMap[m.Name] = true
		}
	}
}
