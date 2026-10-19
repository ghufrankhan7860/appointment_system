package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func TestSwaggerRoute(t *testing.T) {
	tests := []struct {
		name           string
		swaggerEnabled bool
		wantStatus     int
	}{
		{name: "enabled serves the UI", swaggerEnabled: true, wantStatus: http.StatusOK},
		{name: "disabled is not found", swaggerEnabled: false, wantStatus: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpServer, err := New(Config{
				Address:           ":0",
				ReadHeaderTimeout: time.Second,
				ReadTimeout:       time.Second,
				WriteTimeout:      time.Second,
				IdleTimeout:       time.Second,
				ShutdownTimeout:   time.Second,
				SwaggerEnabled:    tt.swaggerEnabled,
			}, healthyChecker{}, noRoutes{})
			if err != nil {
				t.Fatalf("New returned error: %v", err)
			}

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/swagger/index.html", nil)

			httpServer.httpServer.Handler.ServeHTTP(recorder, request)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
		})
	}
}
