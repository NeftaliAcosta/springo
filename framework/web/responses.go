package web

import (
	"encoding/json"
	"encoding/xml"
	"github.com/go-chi/chi/v5"
	"net/http"
	"reflect"
	"strconv"
)

// DecodeJSON parses the request body into a schema
func DecodeJSON(w http.ResponseWriter, r *http.Request, schema interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(schema); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return false
	}
	return true
}

// BindRequest populates a struct with Path and Query parameters based on tags
func BindRequest(r *http.Request, dest interface{}) error {
	val := reflect.ValueOf(dest)
	if val.Kind() != reflect.Ptr || val.Elem().Kind() != reflect.Struct {
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
			setFieldValue(fieldVal, rawValue)
		}
	}
	return nil
}

func setFieldValue(field reflect.Value, value string) {
	if !field.CanSet() {
		return
	}
	switch field.Kind() {
	case reflect.String:
		field.SetString(value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if i, err := strconv.ParseInt(value, 10, 64); err == nil {
			field.SetInt(i)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if i, err := strconv.ParseUint(value, 10, 64); err == nil {
			field.SetUint(i)
		}
	case reflect.Bool:
		if b, err := strconv.ParseBool(value); err == nil {
			field.SetBool(b)
		}
	}
}

// WriteJSON sends a JSON response with a specific status code
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
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
