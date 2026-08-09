package web

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/NeftaliAcosta/springo/framework/config"

	"gopkg.in/yaml.v3"
)

// I18nProperties maps the configuration properties prefix spring.messages
type I18nProperties struct {
	DefaultLocale string `yaml:"default-locale"`
	Path          string `yaml:"path"`
}

func init() {
	config.RegisterProperties("spring.messages", &I18nProperties{
		DefaultLocale: "en",
		Path:          "resources",
	})
}

type contextKey string

// LocaleContextKey is the key to retrieve/store locale in context
const LocaleContextKey contextKey = "locale"

var msgFileRegex = regexp.MustCompile(`^messages(?:_([a-zA-Z_-]+))?\.(?:yaml|yml)$`)

// MessageSource handles properties-based translations with hierarchical fallback
type MessageSource struct {
	defaultLocale string
	translations  map[string]map[string]string // locale -> flattened key -> message
}

// NewMessageSource initializes a new MessageSource instance
func NewMessageSource(defaultLocale string) *MessageSource {
	return &MessageSource{
		defaultLocale: strings.ToLower(defaultLocale),
		translations:  make(map[string]map[string]string),
	}
}

// LoadTranslations scans a directory for translation files and loads them
func (ms *MessageSource) LoadTranslations(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Optional directory
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}

	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		filename := d.Name()
		matches := msgFileRegex.FindStringSubmatch(filename)
		if len(matches) < 2 {
			return nil // Not a message file
		}

		locale := ms.defaultLocale
		if matches[1] != "" {
			locale = strings.ToLower(matches[1])
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", path, err)
		}

		var rawMap map[string]interface{}
		if err := yaml.Unmarshal(content, &rawMap); err != nil {
			return fmt.Errorf("failed to parse YAML message file %s: %w", path, err)
		}

		flattened := make(map[string]string)
		flattenMap("", rawMap, flattened)

		if ms.translations[locale] == nil {
			ms.translations[locale] = make(map[string]string)
		}

		for k, v := range flattened {
			ms.translations[locale][k] = v
		}

		log.Printf("ℹ️ [MessageSource] Loaded %d translations for locale: %s (%s)", len(flattened), locale, filename)
		return nil
	})
}

// GetMessage retrieves and formats a message based on the locale
func (ms *MessageSource) GetMessage(locale string, key string, args ...any) string {
	locale = strings.ToLower(locale)

	// 1. Try exact match
	if msg, found := ms.lookup(locale, key); found {
		return ms.format(msg, args...)
	}

	// 2. Try language fallback (e.g., es-MX -> es)
	if strings.Contains(locale, "-") {
		lang := strings.Split(locale, "-")[0]
		if msg, found := ms.lookup(lang, key); found {
			return ms.format(msg, args...)
		}
	}
	if strings.Contains(locale, "_") {
		lang := strings.Split(locale, "_")[0]
		if msg, found := ms.lookup(lang, key); found {
			return ms.format(msg, args...)
		}
	}

	// 3. Fallback to default locale
	if msg, found := ms.lookup(ms.defaultLocale, key); found {
		return ms.format(msg, args...)
	}

	// 4. Return the key itself formatted
	return ms.format(key, args...)
}

func (ms *MessageSource) lookup(locale, key string) (string, bool) {
	if ms.translations[locale] != nil {
		if msg, ok := ms.translations[locale][key]; ok {
			return msg, true
		}
	}
	return "", false
}

func (ms *MessageSource) format(msg string, args ...any) string {
	if len(args) == 0 {
		return msg
	}
	for i, arg := range args {
		placeholder := fmt.Sprintf("{%d}", i)
		msg = strings.ReplaceAll(msg, placeholder, fmt.Sprint(arg))
	}
	return msg
}

func flattenMap(prefix string, val any, result map[string]string) {
	switch v := val.(type) {
	case map[string]interface{}:
		for k, child := range v {
			newKey := k
			if prefix != "" {
				newKey = prefix + "." + k
			}
			flattenMap(newKey, child, result)
		}
	case map[interface{}]interface{}:
		for k, child := range v {
			keyStr := fmt.Sprintf("%v", k)
			newKey := keyStr
			if prefix != "" {
				newKey = prefix + "." + keyStr
			}
			flattenMap(newKey, child, result)
		}
	default:
		result[prefix] = fmt.Sprintf("%v", v)
	}
}

// GetLocale extracts the locale string from the request context
func GetLocale(ctx context.Context) string {
	if val := ctx.Value(LocaleContextKey); val != nil {
		if locale, ok := val.(string); ok {
			return locale
		}
	}
	return ""
}

// I18nMiddleware parses the locale from query params, Accept-Language headers, or fallback
func I18nMiddleware(defaultLocale string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			locale := ""

			// 1. Check Query Parameter: ?lang=es
			if lang := r.URL.Query().Get("lang"); lang != "" {
				locale = lang
			}

			// 2. Check Accept-Language Header: es-MX,es;q=0.9,en;q=0.8
			if locale == "" {
				if accept := r.Header.Get("Accept-Language"); accept != "" {
					locale = parseAcceptLanguage(accept)
				}
			}

			// 3. Fallback
			if locale == "" {
				locale = defaultLocale
			}

			ctx := context.WithValue(r.Context(), LocaleContextKey, locale)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func parseAcceptLanguage(accept string) string {
	parts := strings.Split(accept, ",")
	if len(parts) > 0 {
		first := strings.TrimSpace(parts[0])
		sub := strings.Split(first, ";")
		if len(sub) > 0 {
			return strings.TrimSpace(sub[0])
		}
	}
	return ""
}
