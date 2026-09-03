package web

import (
	stderrors "errors"
	"mime/multipart"
	"net/http"
	"reflect"

	"github.com/NeftaliAcosta/springo/framework/config"
	frameworkErrors "github.com/NeftaliAcosta/springo/framework/errors"
)

// MultipartFile is the SprinGo equivalent of Spring's MultipartFile. It is an
// alias of multipart.FileHeader, so Open, Filename, Header and Size retain the
// standard library semantics without introducing another file abstraction.
type MultipartFile = multipart.FileHeader

var multipartFileHeaderType = reflect.TypeOf(multipart.FileHeader{})

// BindMultipartRequest binds fields tagged with form into a request DTO.
// Supported file targets are *web.MultipartFile and []*web.MultipartFile;
// scalar form values use the same primitive conversions as path/query binding.
func BindMultipartRequest(w http.ResponseWriter, r *http.Request, dest any) error {
	return bindMultipartRequest(w, r, dest, multipartProperties())
}

func bindMultipartRequest(w http.ResponseWriter, r *http.Request, dest any, props MultipartProperties) error {
	if !props.Enabled {
		return frameworkErrors.BadRequest("Multipart requests are disabled", "MULTIPART_DISABLED")
	}

	r.Body = http.MaxBytesReader(w, r.Body, props.MaxRequestSize)
	if err := r.ParseMultipartForm(props.MemoryThreshold); err != nil {
		var maxBytesErr *http.MaxBytesError
		if stderrors.As(err, &maxBytesErr) {
			return frameworkErrors.PayloadTooLarge("Multipart request exceeds the configured maximum size", "MULTIPART_REQUEST_TOO_LARGE")
		}
		return frameworkErrors.BadRequest("Invalid multipart/form-data payload", "INVALID_MULTIPART_REQUEST")
	}

	value := reflect.ValueOf(dest)
	if value.Kind() != reflect.Pointer || value.IsNil() || value.Elem().Kind() != reflect.Struct {
		return nil
	}
	value = value.Elem()
	typeOfValue := value.Type()
	for i := 0; i < typeOfValue.NumField(); i++ {
		field := typeOfValue.Field(i)
		formName := field.Tag.Get("form")
		if formName == "" || formName == "-" {
			continue
		}
		fieldValue := value.Field(i)
		if !fieldValue.CanSet() {
			continue
		}
		if isMultipartFilePointer(fieldValue.Type()) {
			files := r.MultipartForm.File[formName]
			if len(files) > 0 {
				if err := validateMultipartFileSize(files[0], props.MaxFileSize); err != nil {
					return err
				}
				fieldValue.Set(reflect.ValueOf(files[0]))
			}
			continue
		}
		if isMultipartFileSlice(fieldValue.Type()) {
			files := r.MultipartForm.File[formName]
			for _, file := range files {
				if err := validateMultipartFileSize(file, props.MaxFileSize); err != nil {
					return err
				}
			}
			if len(files) > 0 {
				fieldValue.Set(reflect.ValueOf(files))
			}
			continue
		}
		values := r.MultipartForm.Value[formName]
		if len(values) > 0 {
			setFieldValue(fieldValue, values[0])
		}
	}
	return nil
}

func multipartProperties() MultipartProperties {
	if props := config.Get[WebServerProperties](); props != nil {
		return props.Multipart
	}
	return MultipartProperties{Enabled: true, MaxFileSize: 100 << 20, MaxRequestSize: 110 << 20, MemoryThreshold: 8 << 20}
}

func isMultipartFilePointer(t reflect.Type) bool {
	return t.Kind() == reflect.Pointer && t.Elem() == multipartFileHeaderType
}

func isMultipartFileSlice(t reflect.Type) bool {
	return t.Kind() == reflect.Slice && isMultipartFilePointer(t.Elem())
}

func validateMultipartFileSize(file *multipart.FileHeader, max int64) error {
	if file.Size > max {
		return frameworkErrors.PayloadTooLarge("Multipart file exceeds the configured maximum size", "MULTIPART_FILE_TOO_LARGE")
	}
	return nil
}
