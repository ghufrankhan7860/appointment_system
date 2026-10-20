package middleware_test

import (
	"appointment_system/internal/appcontext"
	"appointment_system/middleware"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type fakeTokenVerifier struct{}

func (f *fakeTokenVerifier) Verify(token string) (uint64, string, error) {
	if token != "good-token" {
		return 0, "", errors.New("invalid token")
	}
	return 7, "patient", nil
}

func TestAuthenticate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name          string
		authorization string
		wantStatus    int
		wantPrincipal string
		wantRole      string
	}{
		{name: "missing header is rejected", authorization: "", wantStatus: http.StatusUnauthorized},
		{name: "non bearer scheme is rejected", authorization: "Basic good-token", wantStatus: http.StatusUnauthorized},
		{name: "invalid token is rejected", authorization: "Bearer bad-token", wantStatus: http.StatusUnauthorized},
		{name: "valid token sets principal", authorization: "Bearer good-token", wantStatus: http.StatusOK, wantPrincipal: "7", wantRole: "patient"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPrincipal, gotRole string

			router := gin.New()
			router.Use(middleware.AppContext(), middleware.Authenticate(&fakeTokenVerifier{}))
			router.GET("/", func(ctx *gin.Context) {
				if applicationContext, ok := appcontext.From(ctx.Request.Context()); ok {
					gotPrincipal = applicationContext.PrincipalID
					gotRole = applicationContext.PrincipalRole
				}
				ctx.Status(http.StatusOK)
			})

			request := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.authorization != "" {
				request.Header.Set("Authorization", tt.authorization)
			}
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, request)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
			if gotPrincipal != tt.wantPrincipal {
				t.Fatalf("PrincipalID = %q, want %q", gotPrincipal, tt.wantPrincipal)
			}
			if gotRole != tt.wantRole {
				t.Fatalf("PrincipalRole = %q, want %q", gotRole, tt.wantRole)
			}
		})
	}
}
