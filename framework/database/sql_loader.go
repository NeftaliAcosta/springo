package database

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/NeftaliAcosta/springo/framework/logging"
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

	slog.Info(fmt.Sprintf("⏳ [SQL Loader] Running %d statements from %s...", len(statements), filepath),
		slog.String(logging.SubsystemKey, logging.FrameworkSubsystem),
		slog.String("file", filepath),
		slog.Int("statements", len(statements)))

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

	slog.Info(fmt.Sprintf("✅ [SQL Loader] Successfully loaded %s", filepath),
		slog.String(logging.SubsystemKey, logging.FrameworkSubsystem),
		slog.String("file", filepath))
	return nil
}

// ParseSQL splits a SQL string into individual statements, ignoring semicolons within quotes and comments.
func ParseSQL(sqlStr string) []string {
	p := newSQLParser(sqlStr)
	return p.parse()
}

type sqlParser struct {
	runes               []rune
	pos                 int
	n                   int
	quoteChar           rune
	inSingleLineComment bool
	inMultiLineComment  bool
	current             strings.Builder
	statements          []string
}

func newSQLParser(sqlStr string) *sqlParser {
	runes := []rune(sqlStr)
	return &sqlParser{
		runes: runes,
		n:     len(runes),
	}
}

func (p *sqlParser) parse() []string {
	for p.pos = 0; p.pos < p.n; p.pos++ {
		r := p.runes[p.pos]
		if p.processSingleLineComment(r) || p.processMultiLineComment(r) || p.tryStartComment(r) {
			continue
		}
		p.updateQuoteState(r)
		if p.handleSemicolon(r) {
			continue
		}
		p.current.WriteRune(r)
	}
	p.flushCurrent()
	return p.statements
}

func (p *sqlParser) processSingleLineComment(r rune) bool {
	if !p.inSingleLineComment {
		return false
	}
	if r == '\n' || r == '\r' {
		p.inSingleLineComment = false
	}
	return true
}

func (p *sqlParser) processMultiLineComment(r rune) bool {
	if !p.inMultiLineComment {
		return false
	}
	if r == '*' && p.peek(1) == '/' {
		p.inMultiLineComment = false
		p.pos++
	}
	return true
}

func (p *sqlParser) tryStartComment(r rune) bool {
	if p.quoteChar != 0 {
		return false
	}
	if r == '-' && p.peek(1) == '-' {
		p.inSingleLineComment = true
		p.pos++
		return true
	}
	if r == '#' {
		p.inSingleLineComment = true
		return true
	}
	if r == '/' && p.peek(1) == '*' {
		p.inMultiLineComment = true
		p.pos++
		return true
	}
	return false
}

func (p *sqlParser) updateQuoteState(r rune) {
	if p.quoteChar == 0 {
		if r == '\'' || r == '"' || r == '`' {
			p.quoteChar = r
		}
		return
	}
	if r == p.quoteChar {
		p.quoteChar = 0
	}
}

func (p *sqlParser) handleSemicolon(r rune) bool {
	if r != ';' || p.quoteChar != 0 {
		return false
	}
	p.flushCurrent()
	return true
}

func (p *sqlParser) flushCurrent() {
	stmt := strings.TrimSpace(p.current.String())
	if stmt != "" {
		p.statements = append(p.statements, stmt)
	}
	p.current.Reset()
}

func (p *sqlParser) peek(offset int) rune {
	idx := p.pos + offset
	if idx < p.n {
		return p.runes[idx]
	}
	return 0
}
