package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func BenchmarkWriteJSON(b *testing.B) {
	payload := ApiResponse[string]{Status: http.StatusOK, Data: "ok"}
	b.ReportAllocs()
	for b.Loop() {
		recorder := httptest.NewRecorder()
		WriteJSON(recorder, http.StatusOK, payload)
	}
}
