package database

import (
	"context"
	"fmt"
	"github.com/NeftaliAcosta/springo/framework/security"
	"reflect"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

var (
	registerAuditOnce sync.Once
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

func registerGormCallbacks(db *gorm.DB) error {
	var initErr error
	registerAuditOnce.Do(func() {
		if err := db.Callback().Create().After("gorm:create").Register("springo:audit_create", auditCreateCallback); err != nil {
			initErr = fmt.Errorf("failed to register audit create callback: %w", err)
			return
		}
		if err := db.Callback().Update().After("gorm:update").Register("springo:audit_update", auditUpdateCallback); err != nil {
			initErr = fmt.Errorf("failed to register audit update callback: %w", err)
			return
		}
		if err := db.Callback().Delete().After("gorm:delete").Register("springo:audit_delete", auditDeleteCallback); err != nil {
			initErr = fmt.Errorf("failed to register audit delete callback: %w", err)
			return
		}
	})
	return initErr
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
	if modelType.Kind() == reflect.Ptr {
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

func isAudited(db *gorm.DB) bool {
	if db.Statement == nil || db.Statement.Schema == nil {
		return false
	}
	isAudited, _, _ := isSchemaAudited(db.Statement.Schema)
	return isAudited
}

func createAuditTable(db *gorm.DB, model interface{}, tableName string) error {
	auditTableName := tableName + "_aud"
	if db.Migrator().HasTable(auditTableName) {
		return nil
	}

	cols, err := db.Migrator().ColumnTypes(model)
	if err != nil {
		return err
	}

	var pkDef string
	switch db.Dialector.Name() {
	case "mysql":
		pkDef = "`audit_id` INT AUTO_INCREMENT PRIMARY KEY"
	default: // sqlite and others
		pkDef = "audit_id INTEGER PRIMARY KEY AUTOINCREMENT"
	}

	sql := fmt.Sprintf("CREATE TABLE %s (%s, rev_type VARCHAR(10), rev_user VARCHAR(255), rev_timestamp DATETIME", auditTableName, pkDef)
	for _, col := range cols {
		if colSql, ok := buildColumnSql(col, db.Dialector.Name()); ok {
			sql += colSql
		}
	}
	sql += ")"

	return db.Exec(sql).Error
}

func formatColumnType(col gorm.ColumnType) string {
	// ColumnType preserves dialect-specific definitions such as MySQL ENUM
	// values. DatabaseTypeName only returns "enum", which produces invalid DDL.
	if fullType, ok := col.ColumnType(); ok && strings.TrimSpace(fullType) != "" {
		return fullType
	}

	dbType := col.DatabaseTypeName()
	dbTypeLower := strings.ToLower(dbType)

	if strings.Contains(dbTypeLower, "char") || strings.Contains(dbTypeLower, "binary") {
		if length, ok := col.Length(); ok && length > 0 {
			return fmt.Sprintf("%s(%d)", dbType, length)
		}
	} else if strings.Contains(dbTypeLower, "decimal") || strings.Contains(dbTypeLower, "numeric") {
		if precision, scale, ok := col.DecimalSize(); ok {
			return fmt.Sprintf("%s(%d,%d)", dbType, precision, scale)
		}
	} else if strings.Contains(dbTypeLower, "datetime") || strings.Contains(dbTypeLower, "timestamp") || strings.Contains(dbTypeLower, "time") {
		if precision, _, ok := col.DecimalSize(); ok && precision > 0 {
			return fmt.Sprintf("%s(%d)", dbType, precision)
		}
	}
	return dbType
}

func buildColumnSql(col gorm.ColumnType, dialector string) (string, bool) {
	colName := col.Name()
	if colName == "audit_id" {
		return "", false
	}
	dbType := formatColumnType(col)
	if dialector == "mysql" {
		return fmt.Sprintf(", `%s` %s", colName, dbType), true
	}
	return fmt.Sprintf(", %s %s", colName, dbType), true
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

func handleAudit(db *gorm.DB, action string) {
	if db.Error != nil {
		return
	}

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
	if destVal.Kind() == reflect.Ptr {
		destVal = destVal.Elem()
	}

	if err := writeDestAudits(db, destVal, tableName, action, userKey); err != nil {
		db.AddError(err) // Abort transaction!
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
	if itemVal.Kind() == reflect.Ptr {
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
		// JSON numbers are parsed as float64, format as integer if no decimal part, or representation
		return fmt.Sprintf("%.0f", v), true
	case bool:
		return fmt.Sprintf("%t", v), true
	default:
		return fmt.Sprintf("%v", v), true
	}
}

func resolveRevUser(ctx context.Context, tableName string, userKey string) (string, error) {
	if ctx == nil {
		if userKey != "" {
			return "", fmt.Errorf("audit error: required user context key '%s' not found for audited entity '%s'", userKey, tableName)
		}
		return "anonymous", nil
	}

	if userKey != "" {
		if val, ok := getContextStringValue(ctx, userKey); ok && val != "" {
			return val, nil
		}
		return "", fmt.Errorf("audit error: required user context key '%s' not found for audited entity '%s'", userKey, tableName)
	}

	// Default mode: try standard context keys
	for _, k := range []interface{}{security.UserContextKey, "username", "user", "user_id", "sub"} {
		if u, ok := getContextStringValue(ctx, k); ok && u != "" {
			return u, nil
		}
	}

	return "anonymous", nil
}

func writeAuditRecord(db *gorm.DB, tableName string, record map[string]interface{}, action string, userKey string) error {
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
