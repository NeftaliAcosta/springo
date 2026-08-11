package web

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	frameworkErrors "github.com/NeftaliAcosta/springo/framework/errors"
	"github.com/go-chi/chi/v5"
)

type multipartRequestDTO struct {
	ResourceUUID string         `path:"resource_uuid" validate:"required"`
	Description  string         `form:"description" validate:"required"`
	File         *MultipartFile `form:"file" validate:"required"`
}

func TestDispatchBindsMultipartDTOAlongsidePathParameters(t *testing.T) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("description", "promotional cover"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("file", "cover.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, "image-content"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	router := chi.NewRouter()
	router.Post("/resources/{resource_uuid}", Dispatch(func(dto multipartRequestDTO) (any, error) {
		if dto.ResourceUUID != "resource-123" || dto.Description != "promotional cover" {
			t.Fatalf("unexpected scalar binding: %+v", dto)
		}
		if dto.File == nil || dto.File.Filename != "cover.png" || dto.File.Size != int64(len("image-content")) {
			t.Fatalf("unexpected file binding: %+v", dto.File)
		}
		file, err := dto.File.Open()
		if err != nil {
			return nil, err
		}
		defer file.Close()
		content, err := io.ReadAll(file)
		if err != nil {
			return nil, err
		}
		return string(content), nil
	}))

	req := httptest.NewRequest(http.MethodPost, "/resources/resource-123", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusCreated || !strings.Contains(recorder.Body.String(), "image-content") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestBindMultipartRequestRejectsOversizedRequest(t *testing.T) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "video.mp4")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(part, strings.Repeat("x", 64))
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	err = bindMultipartRequest(httptest.NewRecorder(), req, &multipartRequestDTO{}, MultipartProperties{
		Enabled: true, MaxFileSize: 32, MaxRequestSize: 48, MemoryThreshold: 8,
	})
	if _, ok := err.(*frameworkErrors.PayloadTooLargeError); !ok {
		t.Fatalf("expected PayloadTooLargeError, got %T: %v", err, err)
	}
}

func TestBindMultipartRequestRejectsOversizedFile(t *testing.T) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "video.mp4")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(part, strings.Repeat("x", 64))
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	err = bindMultipartRequest(httptest.NewRecorder(), req, &multipartRequestDTO{}, MultipartProperties{
		Enabled: true, MaxFileSize: 32, MaxRequestSize: 1024, MemoryThreshold: 8,
	})
	if _, ok := err.(*frameworkErrors.PayloadTooLargeError); !ok {
		t.Fatalf("expected PayloadTooLargeError, got %T: %v", err, err)
	}
}
