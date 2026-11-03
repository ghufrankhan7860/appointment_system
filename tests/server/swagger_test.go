//go:build swagger

package server_test

import (
	"appointment_system/server"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSwaggerRouteServesUI(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/swagger/index.html", nil)

	server.NewRouter(slog.New(slog.DiscardHandler), healthyChecker{}, noRoutes{}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}
