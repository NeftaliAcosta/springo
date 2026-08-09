package web

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/NeftaliAcosta/springo/framework/ioc"

	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	enTranslations "github.com/go-playground/validator/v10/translations/en"
)

var (
	validate *validator.Validate
	trans    ut.Translator
)

func init() {
	initValidator()
}

func initValidator() {
	enLocale := en.New()
	uni := ut.New(enLocale, enLocale)
	trans, _ = uni.GetTranslator("en")

	validate = validator.New()

	// Use json tag names in error messages instead of Go field names
	validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" || name == "" {
			return fld.Name
		}
		return name
	})

	// Register english translations
	_ = enTranslations.RegisterDefaultTranslations(validate, trans)
}

// Validate checks a struct for validation tags.
// Optionally accepts validation groups (structs used as markers, e.g. OnCreate{}, OnUpdate{}).
//
// Group behavior:
//   - No groups → validate ALL fields (legacy behavior preserved).
//   - With groups → only validate fields whose `groups:` tag includes the active group name,
//     OR fields with no `groups:` tag at all (always-on constraints).
//
// DTO field examples:
//
//	Email    string `json:"email"    validate:"required,email"          groups:"OnCreate"`
//	Username string `json:"username" validate:"required,min=3"`          // no groups tag → always validated
//	Password string `json:"password" validate:"required,min=8"           groups:"OnCreate"`
func Validate(s interface{}, groups ...interface{}) error {
	if s == nil {
		return nil
	}

	val := reflect.ValueOf(s)
	if !val.IsValid() {
		return nil
	}

	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return nil
		}
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return nil
	}

	if len(groups) == 0 {
		// No groups: standard full validation
		return validate.StructCtx(context.Background(), s)
	}

	// Build active group name set
	activeGroups := buildGroupSet(groups)

	// Pre-compute excluded namespaces: fields whose groups tag exists but doesn't match active groups.
	// FilterFunc returns TRUE to SKIP (exclude) the field from validation.
	structType := val.Type()
	excludedFields := buildExcludedFieldSet(structType, activeGroups)

	return validate.StructFilteredCtx(context.Background(), s, func(ns []byte) bool {
		// ns format: "StructName.FieldName" — extract field name after last dot
		nsStr := string(ns)
		dotIdx := strings.LastIndex(nsStr, ".")
		fieldName := nsStr
		if dotIdx >= 0 {
			fieldName = nsStr[dotIdx+1:]
		}
		// Return true = SKIP this field (exclude from validation)
		_, skip := excludedFields[fieldName]
		return skip
	})
}

// buildExcludedFieldSet returns field names that should be SKIPPED for the active groups.
// A field is excluded when it has a `groups` tag that does NOT include any active group.
func buildExcludedFieldSet(t reflect.Type, activeGroups map[string]struct{}) map[string]struct{} {
	excluded := map[string]struct{}{}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		groupTag := field.Tag.Get("groups")
		if groupTag == "" {
			// No groups tag → always validate, never exclude
			continue
		}
		fieldBelongsToActiveGroup := false
		for _, g := range strings.Split(groupTag, ",") {
			if _, ok := activeGroups[strings.TrimSpace(g)]; ok {
				fieldBelongsToActiveGroup = true
				break
			}
		}
		if !fieldBelongsToActiveGroup {
			// Use json name if available, else Go field name
			jsonName := strings.SplitN(field.Tag.Get("json"), ",", 2)[0]
			if jsonName == "" || jsonName == "-" {
				jsonName = field.Name
			}
			excluded[jsonName] = struct{}{}
			excluded[field.Name] = struct{}{} // also exclude by Go name (namespace uses Go names)
		}
	}
	return excluded

}

// buildGroupSet converts variadic group values to a set of group type names.
func buildGroupSet(groups []interface{}) map[string]struct{} {
	set := make(map[string]struct{}, len(groups))
	for _, g := range groups {
		name := fmt.Sprintf("%T", g)
		// Strip package prefix: "web.OnCreate" → "OnCreate"
		if idx := strings.LastIndex(name, "."); idx >= 0 {
			name = name[idx+1:]
		}
		set[name] = struct{}{}
	}
	return set
}

// GetValidator returns the underlying validator instance for custom registrations.
func GetValidator() *validator.Validate {
	return validate
}

// GetTranslator returns the active universal translator.
func GetTranslator() ut.Translator {
	return trans
}

// TranslateValidationErrors converts validator.ValidationErrors into a human-readable map.
func TranslateValidationErrors(errs validator.ValidationErrors) map[string]string {
	return TranslateValidationErrorsCtx(context.Background(), errs)
}

// TranslateValidationErrorsCtx converts validator.ValidationErrors into translated error messages based on context locale.
func TranslateValidationErrorsCtx(ctx context.Context, errs validator.ValidationErrors) map[string]string {
	errMap := make(map[string]string, len(errs))
	locale := GetLocale(ctx)

	var ms *MessageSource
	if bean := ioc.GetContainer().GetBean("messageSource"); bean != nil {
		if source, ok := bean.(*MessageSource); ok {
			ms = source
		}
	}

	for _, e := range errs {
		field := e.Field()
		tag := e.Tag()
		param := e.Param()

		translated := ""
		if ms != nil {
			// Find struct name from Namespace if possible (e.g. "UserRequest.email" -> "UserRequest")
			nsParts := strings.Split(e.Namespace(), ".")
			sName := ""
			if len(nsParts) > 1 {
				sName = nsParts[len(nsParts)-2]
			}

			key1 := ""
			if sName != "" {
				key1 = fmt.Sprintf("validation.%s.%s.%s", sName, field, tag)
			}
			key2 := fmt.Sprintf("validation.%s.%s", field, tag)
			key3 := fmt.Sprintf("validation.%s", tag)

			if sName != "" && hasKey(ms, locale, key1) {
				if param != "" {
					translated = ms.GetMessage(locale, key1, param)
				} else {
					translated = ms.GetMessage(locale, key1)
				}
			} else if hasKey(ms, locale, key2) {
				if param != "" {
					translated = ms.GetMessage(locale, key2, param)
				} else {
					translated = ms.GetMessage(locale, key2)
				}
			} else if hasKey(ms, locale, key3) {
				if param != "" {
					translated = ms.GetMessage(locale, key3, param)
				} else {
					translated = ms.GetMessage(locale, key3)
				}
			}
		}

		if translated == "" {
			translated = e.Translate(trans)
		}

		errMap[field] = translated
	}

	return errMap
}

func hasKey(ms *MessageSource, locale, key string) bool {
	locale = strings.ToLower(locale)
	if _, found := ms.lookup(locale, key); found {
		return true
	}
	if strings.Contains(locale, "-") {
		lang := strings.Split(locale, "-")[0]
		if _, found := ms.lookup(lang, key); found {
			return true
		}
	}
	if _, found := ms.lookup(ms.defaultLocale, key); found {
		return true
	}
	return false
}
