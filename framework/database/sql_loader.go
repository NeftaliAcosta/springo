package database

import (
	"fmt"
	"log"
	"os"
	"strings"

	"gorm.io/gorm"
)

// ExecuteSQLFile reads and executes SQL statements from a file inside a transaction.
func ExecuteSQLFile(db *gorm.DB, filepath string) error {
	content, err := os.ReadFile(filepath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // SQL script files are optional
		}
		return fmt.Errorf("failed to read SQL file %s: %w", filepath, err)
	}

	statements := ParseSQL(string(content))
	if len(statements) == 0 {
		return nil
	}

	log.Printf("⏳ [SQL Loader] Running %d statements from %s...", len(statements), filepath)

	// Execute all statements within a single transaction to guarantee atomicity
	err = db.Transaction(func(tx *gorm.DB) error {
		for i, stmt := range statements {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			if err := tx.Exec(stmt).Error; err != nil {
				return fmt.Errorf("statement #%d failed: %q: %w", i+1, stmt, err)
			}
		}
		return nil
	})

	if err != nil {
		return err
	}

	log.Printf("✅ [SQL Loader] Successfully loaded %s", filepath)
	return nil
}

// ParseSQL splits a SQL string into individual statements, ignoring semicolons within quotes and comments.
func ParseSQL(sqlStr string) []string {
	var statements []string
	var current strings.Builder

	inSingleQuote := false
	inDoubleQuote := false
	inBacktick := false
	inSingleLineComment := false
	inMultiLineComment := false

	runes := []rune(sqlStr)
	n := len(runes)

	for i := 0; i < n; i++ {
		r := runes[i]

		// Handle active single-line comment
		if inSingleLineComment {
			if r == '\n' || r == '\r' {
				inSingleLineComment = false
			}
			continue
		}

		// Handle active multi-line comment
		if inMultiLineComment {
			if r == '*' && i+1 < n && runes[i+1] == '/' {
				inMultiLineComment = false
				i++ // Skip the '/' character
			}
			continue
		}

		// Look ahead for comment start markers
		if !inSingleQuote && !inDoubleQuote && !inBacktick {
			if r == '-' && i+1 < n && runes[i+1] == '-' {
				inSingleLineComment = true
				i++
				continue
			}
			if r == '#' {
				inSingleLineComment = true
				continue
			}
			if r == '/' && i+1 < n && runes[i+1] == '*' {
				inMultiLineComment = true
				i++
				continue
			}
		}

		// Toggle string/quoting literal states
		if r == '\'' && !inDoubleQuote && !inBacktick {
			inSingleQuote = !inSingleQuote
		} else if r == '"' && !inSingleQuote && !inBacktick {
			inDoubleQuote = !inDoubleQuote
		} else if r == '`' && !inSingleQuote && !inDoubleQuote {
			inBacktick = !inBacktick
		}

		// Split on semicolon if we are not inside a string literal
		if r == ';' && !inSingleQuote && !inDoubleQuote && !inBacktick {
			stmt := strings.TrimSpace(current.String())
			if stmt != "" {
				statements = append(statements, stmt)
			}
			current.Reset()
			continue
		}

		current.WriteRune(r)
	}

	// Capture any remaining statement after the last semicolon
	stmt := strings.TrimSpace(current.String())
	if stmt != "" {
		statements = append(statements, stmt)
	}

	return statements
}
