package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type healthyChecker struct{}

type noRoutes struct{}

func (healthyChecker) Ping(context.Context) error {
	return nil
}

func (noRoutes) RegisterRoutes(*gin.RouterGroup) {}

func TestHealthRoute(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)

	NewRouter(healthyChecker{}, noRoutes{}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if recorder.Header().Get("X-Correlation-ID") == "" {
		t.Fatal("X-Correlation-ID response header is empty")
	}
	if recorder.Body.String() != "{\"database\":\"ok\",\"status\":\"ok\"}" {
		t.Fatalf("body = %q, want health response", recorder.Body.String())
	}
}
