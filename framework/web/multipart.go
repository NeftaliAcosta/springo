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
	if err := parseMultipartRequest(w, r, props); err != nil {
		return err
	}

	structVal, ok := extractStructValue(dest)
	if !ok {
		return nil
	}

	structType := structVal.Type()
	for i := 0; i < structType.NumField(); i++ {
		if err := bindMultipartField(r, structType.Field(i), structVal.Field(i), props); err != nil {
			return err
		}
	}
	return nil
}

func parseMultipartRequest(w http.ResponseWriter, r *http.Request, props MultipartProperties) error {
	if !props.Enabled {
		return frameworkErrors.BadRequest("Multipart requests are disabled", "MULTIPART_DISABLED")
	}

	r.Body = http.MaxBytesReader(w, r.Body, props.MaxRequestSize)
	if err := r.ParseMultipartForm(props.MemoryThreshold); err != nil {
		var maxBytesErr *http.MaxBytesError
		if stderrors.As(err, &maxBytesErr) {
			return frameworkErrors.PayloadTooLarge(
				"Multipart request exceeds the configured maximum size",
				"MULTIPART_REQUEST_TOO_LARGE",
			)
		}
		return frameworkErrors.BadRequest("Invalid multipart/form-data payload", "INVALID_MULTIPART_REQUEST")
	}
	return nil
}

func extractStructValue(dest any) (reflect.Value, bool) {
	val := reflect.ValueOf(dest)
	if val.Kind() != reflect.Pointer || val.IsNil() {
		return reflect.Value{}, false
	}
	elem := val.Elem()
	if elem.Kind() != reflect.Struct {
		return reflect.Value{}, false
	}
	return elem, true
}

func bindMultipartField(
	r *http.Request,
	field reflect.StructField,
	fieldVal reflect.Value,
	props MultipartProperties,
) error {
	formName := field.Tag.Get("form")
	if formName == "" || formName == "-" || !fieldVal.CanSet() {
		return nil
	}

	fieldType := fieldVal.Type()
	if isMultipartFilePointer(fieldType) {
		return bindSingleFileField(r.MultipartForm.File[formName], fieldVal, props.MaxFileSize)
	}
	if isMultipartFileSlice(fieldType) {
		return bindSliceFileField(r.MultipartForm.File[formName], fieldVal, props.MaxFileSize)
	}
	return bindValueField(r.MultipartForm.Value[formName], fieldVal)
}

func bindSingleFileField(files []*multipart.FileHeader, fieldVal reflect.Value, maxSize int64) error {
	if len(files) == 0 {
		return nil
	}
	if err := validateMultipartFileSize(files[0], maxSize); err != nil {
		return err
	}
	fieldVal.Set(reflect.ValueOf(files[0]))
	return nil
}

func bindSliceFileField(files []*multipart.FileHeader, fieldVal reflect.Value, maxSize int64) error {
	if len(files) == 0 {
		return nil
	}
	for _, file := range files {
		if err := validateMultipartFileSize(file, maxSize); err != nil {
			return err
		}
	}
	fieldVal.Set(reflect.ValueOf(files))
	return nil
}

func bindValueField(values []string, fieldVal reflect.Value) error {
	if len(values) == 0 {
		return nil
	}
	return setFieldValue(fieldVal, values[0])
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
