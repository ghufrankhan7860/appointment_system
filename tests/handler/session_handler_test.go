package handler_test

import (
	"appointment_system/internal/apps/appointment/adapters/payment/none"
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/controller"
	"appointment_system/internal/apps/appointment/handler"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
	"appointment_system/internal/apps/appointment/utils"
	"appointment_system/middleware"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

const sessionJWTSecret = "0123456789abcdef0123456789abcdef"

func newSessionRouter(t *testing.T, sessions *fakeSessionRepo) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	tokens, err := utils.NewJWTManager(sessionJWTSecret, "issuer", time.Minute)
	if err != nil {
		t.Fatalf("NewJWTManager returned error: %v", err)
	}
	authService, err := services.NewAuthService(&fakeUserRepo{}, sessions, tokens, &fakeUserAgentParser{}, discardLogger)
	if err != nil {
		t.Fatalf("NewAuthService returned error: %v", err)
	}
	sessionService, err := services.NewSessionService(sessions, &fakeUserRepo{}, time.Now, discardLogger)
	if err != nil {
		t.Fatalf("NewSessionService returned error: %v", err)
	}
	authController, err := controller.NewAuthController(authService, sessionService)
	if err != nil {
		t.Fatalf("NewAuthController returned error: %v", err)
	}
	authHandler, err := handler.NewAuthHandler(authController, &fakeTokenVerifier{})
	if err != nil {
		t.Fatalf("NewAuthHandler returned error: %v", err)
	}

	router := gin.New()
	router.Use(middleware.AppContext())
	authHandler.RegisterRoutes(router.Group("/api/v1"))

	return router
}

func TestNewAuthHandlerValidation(t *testing.T) {
	if _, err := handler.NewAuthHandler(nil, &fakeTokenVerifier{}); !errors.Is(err, apperrors.ErrNilAuthController) {
		t.Fatalf("nil controller error = %v, want %v", err, apperrors.ErrNilAuthController)
	}

	tokens, err := utils.NewJWTManager(sessionJWTSecret, "issuer", time.Minute)
	if err != nil {
		t.Fatalf("NewJWTManager returned error: %v", err)
	}
	authService, err := services.NewAuthService(&fakeUserRepo{}, &fakeSessionRepo{}, tokens, &fakeUserAgentParser{}, discardLogger)
	if err != nil {
		t.Fatalf("NewAuthService returned error: %v", err)
	}
	if _, err := controller.NewAuthController(authService, nil); !errors.Is(err, apperrors.ErrNilSessionService) {
		t.Fatalf("nil session service error = %v, want %v", err, apperrors.ErrNilSessionService)
	}
	sessionService, err := services.NewSessionService(&fakeSessionRepo{}, &fakeUserRepo{}, time.Now, discardLogger)
	if err != nil {
		t.Fatalf("NewSessionService returned error: %v", err)
	}
	authController, err := controller.NewAuthController(authService, sessionService)
	if err != nil {
		t.Fatalf("NewAuthController returned error: %v", err)
	}
	if _, err := handler.NewAuthHandler(authController, nil); !errors.Is(err, apperrors.ErrNilTokenVerifier) {
		t.Fatalf("nil verifier error = %v, want %v", err, apperrors.ErrNilTokenVerifier)
	}
}

func TestSessionHandlerStatusCodes(t *testing.T) {
	activeSessions := []*models.Session{
		{ID: 21, UserID: 7, DevicePlatform: models.DevicePlatformWeb, ExpiresAt: time.Now().Add(time.Hour)},
		{ID: 22, UserID: 7, DevicePlatform: models.DevicePlatformAndroid, ExpiresAt: time.Now().Add(time.Hour)},
	}

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		token      string
		headers    map[string]string
		sessions   *fakeSessionRepo
		wantStatus int
		wantBody   string
	}{
		{name: "list without a token is unauthorized", method: http.MethodGet, path: "/api/v1/auth/sessions", sessions: &fakeSessionRepo{}, wantStatus: http.StatusUnauthorized},
		{name: "list marks the current session", method: http.MethodGet, path: "/api/v1/auth/sessions", token: "good-token", sessions: &fakeSessionRepo{sessions: activeSessions}, wantStatus: http.StatusOK, wantBody: `"id":21,"current":true`},
		{name: "list leaves other sessions unmarked", method: http.MethodGet, path: "/api/v1/auth/sessions", token: "good-token", sessions: &fakeSessionRepo{sessions: activeSessions}, wantStatus: http.StatusOK, wantBody: `"id":22,"current":false`},
		{name: "revoke with invalid id", method: http.MethodDelete, path: "/api/v1/auth/sessions/abc", token: "good-token", sessions: &fakeSessionRepo{}, wantStatus: http.StatusBadRequest},
		{name: "revoke another user's session", method: http.MethodDelete, path: "/api/v1/auth/sessions/99", token: "good-token", sessions: &fakeSessionRepo{revokeErr: apperrors.ErrSessionNotFound}, wantStatus: http.StatusNotFound},
		{name: "revoke own session", method: http.MethodDelete, path: "/api/v1/auth/sessions/22", token: "good-token", sessions: &fakeSessionRepo{}, wantStatus: http.StatusOK},
		{name: "revoke others keeps the current session", method: http.MethodPost, path: "/api/v1/auth/sessions/revoke-others", token: "good-token", sessions: &fakeSessionRepo{revokedCount: 2}, wantStatus: http.StatusOK, wantBody: `"revoked":2`},
		{name: "login with an unknown platform header", method: http.MethodPost, path: "/api/v1/auth/login", body: `{"email":"a@example.com","password":"password123"}`, headers: map[string]string{"X-Device-Platform": "windows"}, sessions: &fakeSessionRepo{}, wantStatus: http.StatusBadRequest, wantBody: `"error":"Invalid device headers"`},
		{name: "refresh with a long device id header", method: http.MethodPost, path: "/api/v1/auth/refresh", body: `{"token":"refresh-token"}`, headers: map[string]string{"X-Device-Id": strings.Repeat("a", 65)}, sessions: &fakeSessionRepo{}, wantStatus: http.StatusBadRequest, wantBody: `"error":"Invalid device headers"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newSessionRouter(t, tt.sessions)

			request := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "application/json")
			if tt.token != "" {
				request.Header.Set("Authorization", "Bearer "+tt.token)
			}
			for name, value := range tt.headers {
				request.Header.Set(name, value)
			}
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, request)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), tt.wantBody) {
				t.Fatalf("body = %s, want it to contain %s", recorder.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestAdminSessionHandlerStatusCodes(t *testing.T) {
	revokedAt := time.Now().Add(-time.Hour)
	userSessions := []*models.Session{
		{ID: 21, UserID: 7, ExpiresAt: time.Now().Add(time.Hour), UserAgent: "Mozilla/5.0", DeviceID: "device-1"},
		{ID: 20, UserID: 7, ExpiresAt: time.Now().Add(time.Hour), RevokedAt: &revokedAt},
	}

	tests := []struct {
		name       string
		method     string
		path       string
		token      string
		sessions   *fakeSessionRepo
		wantStatus int
		wantBody   string
	}{
		{name: "patient cannot list a user's sessions", method: http.MethodGet, path: "/api/v1/admin/users/7/sessions", token: "patient-token", sessions: &fakeSessionRepo{}, wantStatus: http.StatusForbidden},
		{name: "admin lists sessions with their status", method: http.MethodGet, path: "/api/v1/admin/users/7/sessions", token: "admin-token", sessions: &fakeSessionRepo{sessions: userSessions}, wantStatus: http.StatusOK, wantBody: `"status":"revoked"`},
		{name: "admin sees the raw user agent", method: http.MethodGet, path: "/api/v1/admin/users/7/sessions", token: "admin-token", sessions: &fakeSessionRepo{sessions: userSessions}, wantStatus: http.StatusOK, wantBody: `"status":"active","userAgent":"Mozilla/5.0","deviceId":"device-1"`},
		{name: "unknown user", method: http.MethodGet, path: "/api/v1/admin/users/8/sessions", token: "admin-token", sessions: &fakeSessionRepo{}, wantStatus: http.StatusNotFound},
		{name: "invalid user id", method: http.MethodGet, path: "/api/v1/admin/users/abc/sessions", token: "admin-token", sessions: &fakeSessionRepo{}, wantStatus: http.StatusBadRequest},
		{name: "admin revokes one session", method: http.MethodDelete, path: "/api/v1/admin/users/7/sessions/21", token: "admin-token", sessions: &fakeSessionRepo{}, wantStatus: http.StatusOK},
		{name: "revoking a session that is not the user's", method: http.MethodDelete, path: "/api/v1/admin/users/7/sessions/99", token: "admin-token", sessions: &fakeSessionRepo{revokeErr: apperrors.ErrSessionNotFound}, wantStatus: http.StatusNotFound},
		{name: "admin revokes all sessions", method: http.MethodPost, path: "/api/v1/admin/users/7/sessions/revoke-all", token: "admin-token", sessions: &fakeSessionRepo{revokedCount: 3}, wantStatus: http.StatusOK, wantBody: `"revoked":3`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newDoctorAdminRouterWithSessions(t, &fakeDoctorRepo{}, &fakeAvailabilityRepo{}, &fakePaymentRepo{}, &fakeMedicalRecordRepo{}, none.New(), tt.sessions)

			request := httptest.NewRequest(tt.method, tt.path, nil)
			request.Header.Set("Authorization", "Bearer "+tt.token)
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, request)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), tt.wantBody) {
				t.Fatalf("body = %s, want it to contain %s", recorder.Body.String(), tt.wantBody)
			}
		})
	}
}
