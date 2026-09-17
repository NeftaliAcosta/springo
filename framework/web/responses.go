package web

import (
	stdjson "encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"sync"

	"github.com/NeftaliAcosta/springo/framework/config"
	internaljson "github.com/NeftaliAcosta/springo/framework/internal/json"
	"github.com/go-chi/chi/v5"
)

var (
	jsonEngineOnce sync.Once
	jsonEngineName string
)

func selectedJSONEngine() string {
	jsonEngineOnce.Do(func() {
		jsonEngineName = "standard"
		if props := config.Get[WebServerProperties](); props != nil && props.JSONEngine != "" {
			jsonEngineName = props.JSONEngine
		}
	})
	return jsonEngineName
}

// DefaultMaxJSONBodyBytes sets the maximum allowed payload size for JSON bodies (10MB).
const DefaultMaxJSONBodyBytes int64 = 10 * 1024 * 1024

// DecodeJSON parses the request body into a schema with size limit and single-document enforcement.
func DecodeJSON(w http.ResponseWriter, r *http.Request, schema interface{}) bool {
	if r.Body == nil {
		return true
	}

	r.Body = http.MaxBytesReader(w, r.Body, DefaultMaxJSONBodyBytes)
	var decoder interface{ Decode(any) error }
	if selectedJSONEngine() == "go-json" {
		decoder = internaljson.NewDecoderFor("go-json", r.Body)
	} else {
		decoder = stdjson.NewDecoder(r.Body)
	}

	if err := decoder.Decode(schema); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, "Request body exceeds maximum allowed size", http.StatusRequestEntityTooLarge)
			return false
		}
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return false
	}

	// Verify no trailing extra documents exist in the body
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		http.Error(w, "Request body must contain only a single JSON document", http.StatusBadRequest)
		return false
	}

	return true
}

// BindRequest populates a struct with Path and Query parameters based on tags.
func BindRequest(r *http.Request, dest interface{}) error {
	val := reflect.ValueOf(dest)
	if val.Kind() != reflect.Pointer || val.Elem().Kind() != reflect.Struct {
		return nil // Nothing to bind if it's not a pointer to a struct
	}

	val = val.Elem()
	typ := val.Type()

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		fieldVal := val.Field(i)

		var rawValue string
		if pathTag := field.Tag.Get("path"); pathTag != "" {
			rawValue = chi.URLParam(r, pathTag)
		} else if queryTag := field.Tag.Get("query"); queryTag != "" {
			rawValue = r.URL.Query().Get(queryTag)
		}

		if rawValue != "" {
			if err := setFieldValue(fieldVal, rawValue); err != nil {
				return fmt.Errorf("field %s: %w", field.Name, err)
			}
		}
	}
	return nil
}

func getBitSize(k reflect.Kind) int {
	switch k {
	case reflect.Int8, reflect.Uint8:
		return 8
	case reflect.Int16, reflect.Uint16:
		return 16
	case reflect.Int32, reflect.Uint32:
		return 32
	case reflect.Int64, reflect.Uint64:
		return 64
	default:
		return 0
	}
}

func setFieldValue(field reflect.Value, value string) error {
	if !field.CanSet() {
		return nil
	}
	switch field.Kind() {
	case reflect.String:
		field.SetString(value)
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		bitSize := getBitSize(field.Kind())
		i, err := strconv.ParseInt(value, 10, bitSize)
		if err != nil {
			return fmt.Errorf("invalid integer value %q: %w", value, err)
		}
		field.SetInt(i)
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		bitSize := getBitSize(field.Kind())
		u, err := strconv.ParseUint(value, 10, bitSize)
		if err != nil {
			return fmt.Errorf("invalid unsigned integer value %q: %w", value, err)
		}
		field.SetUint(u)
		return nil
	case reflect.Bool:
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid boolean value %q: %w", value, err)
		}
		field.SetBool(b)
		return nil
	case reflect.Float32, reflect.Float64:
		bitSize := 64
		if field.Kind() == reflect.Float32 {
			bitSize = 32
		}
		f, err := strconv.ParseFloat(value, bitSize)
		if err != nil {
			return fmt.Errorf("invalid float value %q: %w", value, err)
		}
		field.SetFloat(f)
		return nil
	default:
		return nil
	}
}

// WriteJSON sends a JSON response with a specific status code
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if selectedJSONEngine() == "go-json" {
		_ = internaljson.NewEncoderFor("go-json", w).Encode(data)
		return
	}
	_ = stdjson.NewEncoder(w).Encode(data)
}

// ApiResponse is the standard framework wrapper for all API responses
type ApiResponse[T any] struct {
	XMLName xml.Name `json:"-" yaml:"-" xml:"response"`
	Status  int      `json:"status" yaml:"status" xml:"status"`
	Code    string   `json:"code,omitempty" yaml:"code,omitempty" xml:"code,omitempty"`
	Message string   `json:"message,omitempty" yaml:"message,omitempty" xml:"message,omitempty"`
	Data    T        `json:"data,omitempty" yaml:"data,omitempty" xml:"data,omitempty"`
	Errors  any      `json:"errors,omitempty" yaml:"errors,omitempty" xml:"errors,omitempty"`
}

// NewSuccessResponse creates a standard success response
func NewSuccessResponse[T any](status int, data T) ApiResponse[T] {
	return ApiResponse[T]{
		Status: status,
		Data:   data,
	}
}

// NewErrorResponse creates a standard error response
func NewErrorResponse(status int, code string, message string, errors any) ApiResponse[any] {
	return ApiResponse[any]{
		Status:  status,
		Code:    code,
		Message: message,
		Errors:  errors,
	}
}
