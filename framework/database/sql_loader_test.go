package database

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestParseSQL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:  "Simple statements",
			input: "CREATE TABLE users (id INT); INSERT INTO users VALUES (1);",
			expected: []string{
				"CREATE TABLE users (id INT)",
				"INSERT INTO users VALUES (1)",
			},
		},
		{
			name:  "Statements with comments",
			input: "-- This is a comment\nCREATE TABLE users (id INT);\n# Another comment\nINSERT INTO users VALUES (1); /* Inline comment */ SELECT * FROM users;",
			expected: []string{
				"CREATE TABLE users (id INT)",
				"INSERT INTO users VALUES (1)",
				"SELECT * FROM users",
			},
		},
		{
			name:  "Semicolon inside quotes",
			input: "INSERT INTO logs (msg) VALUES ('Hello; World'); INSERT INTO logs (msg) VALUES (\"Double; Quote\");",
			expected: []string{
				"INSERT INTO logs (msg) VALUES ('Hello; World')",
				"INSERT INTO logs (msg) VALUES (\"Double; Quote\")",
			},
		},
		{
			name:  "Multi-line statement",
			input: "CREATE TABLE items (\n  id INT,\n  name VARCHAR(50)\n);",
			expected: []string{
				"CREATE TABLE items (\n  id INT,\n  name VARCHAR(50)\n)",
			},
		},
		{
			name:  "No trailing semicolon",
			input: "SELECT * FROM users",
			expected: []string{
				"SELECT * FROM users",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := ParseSQL(tt.input)
			assert.Equal(t, tt.expected, output)
		})
	}
}

func TestExecuteSQLFile(t *testing.T) {
	// Create an in-memory SQLite DB
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	tmpDir := t.TempDir()

	schemaPath := filepath.Join(tmpDir, "schema.sql")
	dataPath := filepath.Join(tmpDir, "data.sql")

	schemaContent := `
	CREATE TABLE test_users (
		id INTEGER PRIMARY KEY,
		name TEXT,
		email TEXT
	);
	`
	dataContent := `
	INSERT INTO test_users (id, name, email) VALUES (1, 'John Doe', 'john;doe@example.com');
	INSERT INTO test_users (id, name, email) VALUES (2, 'Jane Doe', 'jane@example.com');
	`

	err = os.WriteFile(schemaPath, []byte(schemaContent), 0644)
	require.NoError(t, err)

	err = os.WriteFile(dataPath, []byte(dataContent), 0644)
	require.NoError(t, err)

	// Execute schema
	err = ExecuteSQLFile(db, schemaPath)
	require.NoError(t, err)

	// Execute data
	err = ExecuteSQLFile(db, dataPath)
	require.NoError(t, err)

	// Verify data
	type TestUser struct {
		ID    int
		Name  string
		Email string
	}

	var users []TestUser
	err = db.Find(&users).Error
	require.NoError(t, err)

	require.Len(t, users, 2)
	assert.Equal(t, "John Doe", users[0].Name)
	assert.Equal(t, "john;doe@example.com", users[0].Email)
	assert.Equal(t, "Jane Doe", users[1].Name)
}

func TestExecuteSQLFile_TransactionRollback(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	// Set up table
	err = db.Exec("CREATE TABLE test_entities (id INTEGER PRIMARY KEY, code TEXT UNIQUE);").Error
	require.NoError(t, err)

	tmpDir := t.TempDir()

	dataPath := filepath.Join(tmpDir, "data.sql")

	// The second insert violates UNIQUE constraint on 'code'
	badDataContent := `
	INSERT INTO test_entities (id, code) VALUES (1, 'A');
	INSERT INTO test_entities (id, code) VALUES (2, 'A');
	`
	err = os.WriteFile(dataPath, []byte(badDataContent), 0644)
	require.NoError(t, err)

	err = ExecuteSQLFile(db, dataPath)
	require.Error(t, err) // Should fail due to constraint

	// Verify nothing was inserted (rollback worked)
	var count int64
	err = db.Table("test_entities").Count(&count).Error
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}
