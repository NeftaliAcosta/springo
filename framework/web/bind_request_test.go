package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	frameworkErrors "github.com/NeftaliAcosta/springo/framework/errors"
	"github.com/go-chi/chi/v5"
)

type pagedRequestDTO struct {
	ID   int `path:"id"`
	Page int `query:"page"`
}

func TestBindRequestRejectsUnconvertibleValuesWithBadRequest(t *testing.T) {
	err := BindRequest(httptest.NewRequest(http.MethodGet, "/items?page=abc", nil), &pagedRequestDTO{})
	var badRequest *frameworkErrors.BadRequestError
	if !errors.As(err, &badRequest) || badRequest.Code != "INVALID_PARAMETER" {
		t.Fatalf("expected INVALID_PARAMETER bad request, got %#v", err)
	}
	if strings.Contains(badRequest.Message, "abc") {
		t.Fatalf("the raw value must not be echoed: %q", badRequest.Message)
	}
}

func TestDispatchAnswersInvalidParametersWith400(t *testing.T) {
	router := chi.NewRouter()
	router.Get("/items/{id}", Dispatch(func(dto pagedRequestDTO) (any, error) {
		return dto, nil
	}, WithSuccessStatus(http.StatusOK)))

	for path, want := range map[string]int{
		"/items/7?page=2":   http.StatusOK,
		"/items/7?page=abc": http.StatusBadRequest,
		"/items/x?page=2":   http.StatusBadRequest,
	} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != want {
			t.Fatalf("%s: status=%d want %d body=%s", path, recorder.Code, want, recorder.Body.String())
		}
		if want == http.StatusBadRequest && !strings.Contains(recorder.Body.String(), "INVALID_PARAMETER") {
			t.Fatalf("%s: body=%s", path, recorder.Body.String())
		}
	}
}
