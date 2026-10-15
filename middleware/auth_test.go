package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequireAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	verify := func(token string) (uint64, error) {
		if token == "good" {
			return 7, nil
		}
		return 0, errors.New("bad token")
	}

	tests := []struct {
		name   string
		header string
		want   int
	}{
		{"missing header", "", http.StatusUnauthorized},
		{"wrong scheme", "Basic good", http.StatusUnauthorized},
		{"invalid token", "Bearer nope", http.StatusUnauthorized},
		{"valid token", "Bearer good", http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/", RequireAuth(verify), func(ctx *gin.Context) {
				if id, ok := UserID(ctx); !ok || id != 7 {
					t.Errorf("UserID = %d, %v; want 7, true", id, ok)
				}
				ctx.Status(http.StatusOK)
			})
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}
