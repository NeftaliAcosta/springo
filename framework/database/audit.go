package database

import (
	"context"
	"fmt"
	"github.com/NeftaliAcosta/springo/framework/security"
	"reflect"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)



// EnableAuditing configures GORM callbacks and creates the necessary *_aud tables
// for any models marked with the `springo:"audited"` struct tag.
func EnableAuditing(db *gorm.DB, models ...interface{}) error {
	if err := registerGormCallbacks(db); err != nil {
		return err
	}

	for _, model := range models {
		if err := setupModelAuditing(db, model); err != nil {
			return fmt.Errorf("failed to enable database auditing: %w", err)
		}
	}

	return nil
}

// RegisterGormCallbacks registers audit hooks for create, update, and delete on the given DB handle.
func registerGormCallbacks(db *gorm.DB) error {
	cb := db.Callback()

	err := cb.Create().
		After("gorm:create").
		Register("springo:audit_create", auditCreateCallback)
	if err != nil {
		return fmt.Errorf("failed to register audit create callback: %w", err)
	}

	err = cb.Update().
		After("gorm:update").
		Register("springo:audit_update", auditUpdateCallback)
	if err != nil {
		return fmt.Errorf("failed to register audit update callback: %w", err)
	}

	err = cb.Delete().
		After("gorm:delete").
		Register("springo:audit_delete", auditDeleteCallback)
	if err != nil {
		return fmt.Errorf("failed to register audit delete callback: %w", err)
	}

	return nil
}

func setupModelAuditing(db *gorm.DB, model interface{}) error {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(model); err != nil {
		return fmt.Errorf("failed to parse schema for model: %w", err)
	}

	isAudited, _, err := isSchemaAudited(stmt.Schema)
	if err != nil {
		return fmt.Errorf("invalid tag configuration on model %T: %w", model, err)
	}

	if isAudited {
		tableName := stmt.Schema.Table
		if err := createAuditTable(db, model, tableName); err != nil {
			return fmt.Errorf("failed to create audit table for %s: %w", tableName, err)
		}
	}

	return nil
}

func parseAuditTag(tagValue string) (isAudited bool, userKey string, err error) {
	parts := strings.Split(tagValue, ";")
	if len(parts) == 0 || parts[0] != "audited" {
		return false, "", nil
	}
	isAudited = true
	for _, part := range parts[1:] {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			return true, "", fmt.Errorf("invalid parameter format '%s' (must be key=value)", part)
		}
		key := strings.TrimSpace(kv[0])
		val := strings.TrimSpace(kv[1])

		if key != "user_key" {
			return true, "", fmt.Errorf("unknown parameter '%s'", key)
		}
		if val == "" {
			return true, "", fmt.Errorf("parameter 'user_key' cannot be empty")
		}
		userKey = val
	}
	return isAudited, userKey, nil
}

func isSchemaAudited(sch *schema.Schema) (bool, string, error) {
	if sch == nil {
		return false, "", nil
	}
	modelType := sch.ModelType
	if modelType.Kind() == reflect.Pointer {
		modelType = modelType.Elem()
	}
	if modelType.Kind() != reflect.Struct {
		return false, "", nil
	}
	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)
		val, ok := field.Tag.Lookup("springo")
		if !ok {
			val, ok = field.Tag.Lookup("github.com/NeftaliAcosta/springo")
		}
		if ok {
			isAudited, userKey, err := parseAuditTag(val)
			if err != nil {
				return false, "", fmt.Errorf("field '%s': %w", field.Name, err)
			}
			if isAudited {
				return true, userKey, nil
			}
		}
	}
	return false, "", nil
}



// GetAuditPKDef returns the dialect-appropriate primary key definition string.
func getAuditPKDef(dialector string) string {
	switch dialector {
	case "mysql":
		return "`audit_id` INT AUTO_INCREMENT PRIMARY KEY"
	case "postgres":
		return `"audit_id" BIGSERIAL PRIMARY KEY`
	default: // sqlite and fallbacks
		return "audit_id INTEGER PRIMARY KEY AUTOINCREMENT"
	}
}

// GetAuditTimestampType returns the dialect-appropriate timestamp type name.
func getAuditTimestampType(dialector string) string {
	if dialector == "postgres" {
		return "TIMESTAMPTZ"
	}
	return "DATETIME"
}

// QuoteIdentifier quotes table and column names according to the target SQL dialect.
func quoteIdentifier(dialector string, identifier string) string {
	switch dialector {
	case "mysql":
		return fmt.Sprintf("`%s`", identifier)
	case "postgres":
		return fmt.Sprintf(`"%s"`, identifier)
	default: // sqlite
		return identifier
	}
}

// CreateAuditTable generates and executes dialect-compatible DDL for audit tables.
func createAuditTable(db *gorm.DB, model interface{}, tableName string) error {
	dialector := db.Name()
	auditTableName := quoteIdentifier(dialector, tableName+"_aud")

	if db.Migrator().HasTable(tableName + "_aud") {
		return nil
	}

	cols, err := db.Migrator().ColumnTypes(model)
	if err != nil {
		return err
	}

	pkDef := getAuditPKDef(dialector)
	tsType := getAuditTimestampType(dialector)

	revTypeCol := quoteIdentifier(dialector, "rev_type")
	revUserCol := quoteIdentifier(dialector, "rev_user")
	revTimestampCol := quoteIdentifier(dialector, "rev_timestamp")

	sql := fmt.Sprintf(
		"CREATE TABLE %s (%s, %s VARCHAR(10), %s VARCHAR(255), %s %s",
		auditTableName,
		pkDef,
		revTypeCol,
		revUserCol,
		revTimestampCol,
		tsType,
	)

	for _, col := range cols {
		if colSql, ok := buildColumnSql(col, dialector); ok {
			sql += colSql
		}
	}
	sql += ")"

	return db.Exec(sql).Error
}

// FormatColumnType formats dialect-specific column definitions for DDL table creation.
func formatColumnType(col gorm.ColumnType) string {
	if fullType, ok := col.ColumnType(); ok && strings.TrimSpace(fullType) != "" {
		return fullType
	}

	dbType := col.DatabaseTypeName()
	dbTypeLower := strings.ToLower(dbType)

	if formatted, ok := formatCharOrBinaryType(col, dbType, dbTypeLower); ok {
		return formatted
	}

	if formatted, ok := formatDecimalType(col, dbType, dbTypeLower); ok {
		return formatted
	}

	if formatted, ok := formatDateTimeType(col, dbType, dbTypeLower); ok {
		return formatted
	}

	return dbType
}

// FormatCharOrBinaryType formats CHAR, VARCHAR, or BINARY column types with length specifiers.
func formatCharOrBinaryType(col gorm.ColumnType, dbType string, dbTypeLower string) (string, bool) {
	if !strings.Contains(dbTypeLower, "char") && !strings.Contains(dbTypeLower, "binary") {
		return "", false
	}
	if length, ok := col.Length(); ok && length > 0 {
		return fmt.Sprintf("%s(%d)", dbType, length), true
	}
	return "", false
}

// FormatDecimalType formats DECIMAL or NUMERIC column types with precision and scale.
func formatDecimalType(col gorm.ColumnType, dbType string, dbTypeLower string) (string, bool) {
	if !strings.Contains(dbTypeLower, "decimal") && !strings.Contains(dbTypeLower, "numeric") {
		return "", false
	}
	if precision, scale, ok := col.DecimalSize(); ok {
		return fmt.Sprintf("%s(%d,%d)", dbType, precision, scale), true
	}
	return "", false
}

// FormatDateTimeType formats DATETIME, TIMESTAMP, or TIME column types with precision.
func formatDateTimeType(col gorm.ColumnType, dbType string, dbTypeLower string) (string, bool) {
	if !strings.Contains(dbTypeLower, "datetime") &&
		!strings.Contains(dbTypeLower, "timestamp") &&
		!strings.Contains(dbTypeLower, "time") {
		return "", false
	}
	if precision, _, ok := col.DecimalSize(); ok && precision > 0 {
		return fmt.Sprintf("%s(%d)", dbType, precision), true
	}
	return "", false
}

// BuildColumnSql constructs the SQL column definition fragment for audit tables.
func buildColumnSql(col gorm.ColumnType, dialector string) (string, bool) {
	colName := col.Name()
	if colName == "audit_id" {
		return "", false
	}

	quotedColName := quoteIdentifier(dialector, colName)
	dbType := formatColumnType(col)

	return fmt.Sprintf(", %s %s", quotedColName, dbType), true
}

func auditCreateCallback(db *gorm.DB) {
	handleAudit(db, "INSERT")
}

func auditUpdateCallback(db *gorm.DB) {
	handleAudit(db, "UPDATE")
}

func auditDeleteCallback(db *gorm.DB) {
	handleAudit(db, "DELETE")
}

// HandleAudit intercepts GORM create, update, and delete callbacks to produce audit records safely.
func handleAudit(db *gorm.DB, action string) {
	if db.Error != nil {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			_ = db.AddError(fmt.Errorf("audit callback panic: %v", r))
		}
	}()

	if strings.HasSuffix(db.Statement.Table, "_aud") || strings.HasSuffix(getTableName(db), "_aud") {
		return
	}

	if db.Statement.Schema == nil {
		return
	}

	isAudited, userKey, _ := isSchemaAudited(db.Statement.Schema)
	if !isAudited {
		return
	}

	tableName := db.Statement.Schema.Table
	destVal := reflect.ValueOf(db.Statement.Dest)
	if destVal.Kind() == reflect.Pointer {
		destVal = destVal.Elem()
	}

	if err := writeDestAudits(db, destVal, tableName, action, userKey); err != nil {
		_ = db.AddError(err)
	}
}

func writeDestAudits(db *gorm.DB, destVal reflect.Value, tableName string, action string, userKey string) error {
	if destVal.Kind() == reflect.Slice || destVal.Kind() == reflect.Array {
		for i := 0; i < destVal.Len(); i++ {
			record := getRecordMap(db, destVal.Index(i))
			if err := writeAuditRecord(db, tableName, record, action, userKey); err != nil {
				return err
			}
		}
		return nil
	}

	record := getRecordMap(db, destVal)
	return writeAuditRecord(db, tableName, record, action, userKey)
}

func getTableName(db *gorm.DB) string {
	if db.Statement.Schema != nil {
		return db.Statement.Schema.Table
	}
	return db.Statement.Table
}

func getRecordMap(db *gorm.DB, itemVal reflect.Value) map[string]interface{} {
	if itemVal.Kind() == reflect.Pointer {
		itemVal = itemVal.Elem()
	}
	record := make(map[string]interface{})
	if itemVal.Kind() != reflect.Struct {
		return record
	}
	for _, field := range db.Statement.Schema.Fields {
		if field.DBName == "" {
			continue
		}
		val, _ := field.ValueOf(context.Background(), itemVal)
		record[field.DBName] = val
	}
	return record
}

// GetContextStringValue extracts and formats string representation of context values for key.
func getContextStringValue(ctx context.Context, key interface{}) (string, bool) {
	if ctx == nil {
		return "", false
	}
	val := ctx.Value(key)
	if val == nil {
		return "", false
	}
	switch v := val.(type) {
	case string:
		return v, true
	case int:
		return fmt.Sprintf("%d", v), true
	case int64:
		return fmt.Sprintf("%d", v), true
	case float64:
		return fmt.Sprintf("%.0f", v), true
	case bool:
		return fmt.Sprintf("%t", v), true
	default:
		return fmt.Sprintf("%v", v), true
	}
}

// SanitizeRevUser validates, cleans, and truncates the username string for audit persistence.
func sanitizeRevUser(raw string) string {
	cleaned := strings.TrimSpace(raw)
	if cleaned == "" {
		return "anonymous"
	}

	var builder strings.Builder
	builder.Grow(len(cleaned))
	for _, r := range cleaned {
		if r == '\n' || r == '\r' || r == '\t' {
			builder.WriteRune(' ')
			continue
		}
		if r < 0x20 || r == 0x7F {
			continue
		}
		builder.WriteRune(r)
	}

	fields := strings.Fields(builder.String())
	if len(fields) == 0 {
		return "anonymous"
	}
	result := strings.Join(fields, " ")

	runes := []rune(result)
	if len(runes) > 255 {
		return string(runes[:255])
	}

	return result
}

// ResolveRevUser extracts and sanitizes the audit user identifier from the request context.
func resolveRevUser(ctx context.Context, tableName string, userKey string) (string, error) {
	if ctx == nil {
		if userKey != "" {
			return "", fmt.Errorf(
				"audit error: required user context key '%s' not found for audited entity '%s'",
				userKey,
				tableName,
			)
		}
		return "anonymous", nil
	}

	if userKey != "" {
		return resolveUserKey(ctx, userKey, tableName)
	}

	return resolveDefaultUser(ctx), nil
}

// ResolveUserKey extracts and sanitizes a user identifier using a specific required context key.
func resolveUserKey(ctx context.Context, userKey string, tableName string) (string, error) {
	if val, ok := getContextStringValue(ctx, userKey); ok && val != "" {
		sanitized := sanitizeRevUser(val)
		if sanitized != "anonymous" {
			return sanitized, nil
		}
	}

	return "", fmt.Errorf(
		"audit error: required user context key '%s' not found for audited entity '%s'",
		userKey,
		tableName,
	)
}

// ResolveDefaultUser iterates standard context keys to extract and sanitize a user identifier.
func resolveDefaultUser(ctx context.Context) string {
	keys := []interface{}{security.UserContextKey, "username", "user", "user_id", "sub"}
	for _, k := range keys {
		u, ok := getContextStringValue(ctx, k)
		if !ok || u == "" {
			continue
		}

		sanitized := sanitizeRevUser(u)
		if sanitized != "anonymous" {
			return sanitized
		}
	}

	return "anonymous"
}

// WriteAuditRecord persists an audit entry into the target entity's _aud table.
func writeAuditRecord(
	db *gorm.DB,
	tableName string,
	record map[string]interface{},
	action string,
	userKey string,
) error {
	auditTableName := tableName + "_aud"

	username, err := resolveRevUser(db.Statement.Context, tableName, userKey)
	if err != nil {
		return err
	}

	record["rev_type"] = action
	record["rev_user"] = username
	record["rev_timestamp"] = time.Now()

	// Ensure we do not pass custom primary key field if GORM tries to inject it
	delete(record, "audit_id")

	// Use Session with NewDB: true to get a clean builder
	err = db.Session(&gorm.Session{NewDB: true}).Table(auditTableName).Create(record).Error
	if err != nil {
		return fmt.Errorf("failed to write audit record to %s: %w", auditTableName, err)
	}

	return nil
}
