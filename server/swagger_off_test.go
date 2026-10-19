//go:build !swagger

package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSwaggerRouteAbsentWithoutTag(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/swagger/index.html", nil)

	NewRouter(healthyChecker{}, noRoutes{}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}
