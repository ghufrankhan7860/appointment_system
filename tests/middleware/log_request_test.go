package middleware_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"appointment_system/internal/platform/logger"
	"appointment_system/middleware"

	"github.com/gin-gonic/gin"
)

func newTestRouter(t *testing.T, output *bytes.Buffer) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	jsonLogger, err := logger.New(logger.Config{Level: "info", Format: logger.FormatJSON}, output)
	if err != nil {
		t.Fatalf("logger.New returned error: %v", err)
	}

	router := gin.New()
	router.Use(middleware.AppContext(), middleware.LogRequest(jsonLogger))
	adminRoutes := router.Group("/admin", middleware.Authenticate(&fakeTokenVerifier{}), middleware.LogAdminRequest(jsonLogger))
	adminRoutes.GET("/doctors/:id", func(ctx *gin.Context) { ctx.Status(http.StatusOK) })
	router.GET("/appointments/:id", func(ctx *gin.Context) {
		switch ctx.Param("id") {
		case "500":
			_ = ctx.Error(errors.New("get appointment by id: connection refused"))
			ctx.Status(http.StatusInternalServerError)
		case "400":
			ctx.Status(http.StatusBadRequest)
		default:
			ctx.Status(http.StatusOK)
		}
	})

	return router
}

func logLines(t *testing.T, output *bytes.Buffer) []map[string]any {
	t.Helper()

	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("log line %q is not JSON: %v", line, err)
		}
		records = append(records, record)
	}

	return records
}

func TestLogRequest(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		wantLevel string
		wantRoute string
		wantError string
	}{
		{name: "success is info", path: "/appointments/42", wantLevel: "INFO", wantRoute: "/appointments/:id"},
		{name: "client error is warn", path: "/appointments/400", wantLevel: "WARN", wantRoute: "/appointments/:id"},
		{name: "server error carries the cause", path: "/appointments/500", wantLevel: "ERROR", wantRoute: "/appointments/:id", wantError: "connection refused"},
		{name: "unmatched path is not logged raw", path: "/patients/9/secret", wantLevel: "WARN", wantRoute: "unmatched"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			router := newTestRouter(t, &output)

			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			request.Header.Set("X-Correlation-ID", "corr-1")
			router.ServeHTTP(httptest.NewRecorder(), request)

			records := logLines(t, &output)
			if len(records) != 1 {
				t.Fatalf("logged %d lines, want 1: %s", len(records), output.String())
			}
			record := records[0]
			if record["msg"] != "request served" || record["level"] != tt.wantLevel || record["route"] != tt.wantRoute ||
				record["correlation_id"] != "corr-1" || record["method"] != http.MethodGet {
				t.Fatalf("record = %v, want %s %s with the correlation ID", record, tt.wantLevel, tt.wantRoute)
			}
			gotError, _ := record["error"].(string)
			if tt.wantError == "" && gotError != "" || !strings.Contains(gotError, tt.wantError) {
				t.Fatalf("error = %q, want %q", gotError, tt.wantError)
			}
			if strings.Contains(output.String(), tt.path) && tt.path != tt.wantRoute {
				t.Fatalf("output %q contains the raw path", output.String())
			}
		})
	}
}

func TestLogAdminRequest(t *testing.T) {
	var output bytes.Buffer
	router := newTestRouter(t, &output)

	request := httptest.NewRequest(http.MethodGet, "/admin/doctors/5", nil)
	request.Header.Set("Authorization", "Bearer good-token")
	router.ServeHTTP(httptest.NewRecorder(), request)

	var audit map[string]any
	for _, record := range logLines(t, &output) {
		if record["msg"] == "admin request" {
			audit = record
		}
	}
	if audit == nil {
		t.Fatalf("no admin request line in %s", output.String())
	}
	if audit["audit"] != true || audit["user_id"] != "7" || audit["route"] != "/admin/doctors/:id" || audit["status"] != float64(http.StatusOK) {
		t.Fatalf("admin request line = %v, want an audit line with the user, route pattern and status", audit)
	}
}
