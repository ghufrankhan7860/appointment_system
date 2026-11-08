package services_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
	"appointment_system/internal/apps/appointment/utils"
	"appointment_system/internal/platform/logger"
)

func TestAuthServiceRefreshSessionState(t *testing.T) {
	revokedAt := time.Now().Add(-time.Hour)

	tests := []struct {
		name    string
		session *models.Session
		wantErr error
	}{
		{
			name: "revoked session is rejected",
			session: &models.Session{
				ID:        1,
				UserID:    7,
				TokenHash: "hash",
				ExpiresAt: time.Now().Add(time.Hour),
				RevokedAt: &revokedAt,
			},
			wantErr: apperrors.ErrSessionRevoked,
		},
		{
			name: "expired session is rejected",
			session: &models.Session{
				ID:        2,
				UserID:    7,
				TokenHash: "hash",
				ExpiresAt: time.Now().Add(-time.Hour),
				RevokedAt: nil,
			},
			wantErr: apperrors.ErrSessionExpired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessions := &fakeSessionRepo{session: tt.session}
			service, err := services.NewAuthService(
				&fakeUserRepo{user: &models.User{ID: 7}},
				sessions,
				&fakeTokenGen{token: "access-token"},
				&fakeUserAgentParser{},
				discardLogger,
			)
			if err != nil {
				t.Fatalf("NewAuthService returned error: %v", err)
			}

			_, _, err = service.Refresh(context.Background(), "refresh-token", models.SessionClient{})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Refresh error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestAuthServiceRefreshRotatesInPlace(t *testing.T) {
	sessions := &fakeSessionRepo{
		session: &models.Session{
			ID:         3,
			UserID:     7,
			TokenHash:  "hash",
			ExpiresAt:  time.Now().Add(time.Hour),
			IPAddress:  "203.0.113.10",
			DeviceID:   "device-1",
			AppVersion: "1.0.0",
			Timezone:   "Asia/Kolkata",
		},
	}
	tokens := &fakeTokenGen{token: "access-token"}
	parser := &fakeUserAgentParser{details: models.UserAgentDetails{BrowserName: "Chrome", BrowserVersion: "131.0"}}
	service, err := services.NewAuthService(&fakeUserRepo{user: &models.User{ID: 7}}, sessions, tokens, parser, discardLogger)
	if err != nil {
		t.Fatalf("NewAuthService returned error: %v", err)
	}

	accessToken, nextRefreshToken, err := service.Refresh(context.Background(), "refresh-token", models.SessionClient{
		IPAddress:  "198.51.100.7",
		UserAgent:  "Mozilla/5.0 Chrome/131.0",
		DeviceID:   "device-1",
		AppVersion: "1.1.0",
	})
	if err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}
	if accessToken == "" || nextRefreshToken == "" {
		t.Fatalf("Refresh returned empty tokens: %q, %q", accessToken, nextRefreshToken)
	}
	if sessions.createCalled || sessions.revokeCalled {
		t.Fatalf("Refresh created or revoked a session, want the same session rotated in place")
	}
	rotated := sessions.rotated
	if rotated == nil || sessions.rotatedFrom != "hash" || rotated.TokenHash != utils.HashToken(nextRefreshToken) {
		t.Fatalf("Refresh rotated %+v from %q, want the new token hash replacing hash", rotated, sessions.rotatedFrom)
	}
	if rotated.LastSeenIP != "198.51.100.7" || rotated.IPAddress != "203.0.113.10" || rotated.DeviceID != "device-1" {
		t.Fatalf("Refresh rotated IPs %q/%q device %q, want last seen updated and login IP and device kept", rotated.LastSeenIP, rotated.IPAddress, rotated.DeviceID)
	}
	if rotated.AppVersion != "1.1.0" || rotated.Timezone != "Asia/Kolkata" || rotated.Browser.BrowserVersion != "131.0" {
		t.Fatalf("Refresh rotated app %q timezone %q browser %+v, want the new app version and browser and the old timezone", rotated.AppVersion, rotated.Timezone, rotated.Browser)
	}
	if tokens.sessionID != 3 {
		t.Fatalf("Refresh issued a token for session %d, want 3", tokens.sessionID)
	}
}

func TestAuthServiceLogoutRevokedSessionIsIdempotent(t *testing.T) {
	revokedAt := time.Now().Add(-time.Hour)
	sessions := &fakeSessionRepo{
		session: &models.Session{
			ID:        4,
			UserID:    7,
			TokenHash: "hash",
			ExpiresAt: time.Now().Add(time.Hour),
			RevokedAt: &revokedAt,
		},
	}
	service, err := services.NewAuthService(
		&fakeUserRepo{},
		sessions,
		&fakeTokenGen{},
		&fakeUserAgentParser{},
		discardLogger,
	)
	if err != nil {
		t.Fatalf("NewAuthService returned error: %v", err)
	}

	if err := service.Logout(context.Background(), "refresh-token"); err != nil {
		t.Fatalf("Logout returned error: %v", err)
	}
	if sessions.revokeCalled {
		t.Fatalf("Logout revoked an already-revoked session")
	}
}

func TestNewAuthServiceValidation(t *testing.T) {
	parser := &fakeUserAgentParser{}
	if _, err := services.NewAuthService(nil, &fakeSessionRepo{}, &fakeTokenGen{}, parser, discardLogger); !errors.Is(err, apperrors.ErrNilUserRepository) {
		t.Fatalf("nil users error = %v", err)
	}
	if _, err := services.NewAuthService(&fakeUserRepo{}, nil, &fakeTokenGen{}, parser, discardLogger); !errors.Is(err, apperrors.ErrNilSessionRepository) {
		t.Fatalf("nil sessions error = %v", err)
	}
	if _, err := services.NewAuthService(&fakeUserRepo{}, &fakeSessionRepo{}, nil, parser, discardLogger); !errors.Is(err, apperrors.ErrNilTokenGenerator) {
		t.Fatalf("nil tokens error = %v", err)
	}
	if _, err := services.NewAuthService(&fakeUserRepo{}, &fakeSessionRepo{}, &fakeTokenGen{}, nil, discardLogger); !errors.Is(err, apperrors.ErrNilUserAgentParser) {
		t.Fatalf("nil parser error = %v", err)
	}
}

func TestAuthServiceRegisterReportsDuplicateField(t *testing.T) {
	tests := []struct {
		name       string
		repoErr    error
		wantErr    error
		wantReason string
	}{
		{name: "email taken", repoErr: apperrors.ErrEmailAlreadyExists, wantErr: apperrors.ErrEmailAlreadyExists, wantReason: "email is already registered"},
		{name: "mobile number taken", repoErr: apperrors.ErrMobileAlreadyExists, wantErr: apperrors.ErrMobileAlreadyExists, wantReason: "mobileNo is already registered"},
		{name: "unknown clash keeps the generic error", repoErr: apperrors.ErrUserAlreadyExists, wantErr: apperrors.ErrUserAlreadyExists, wantReason: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, err := services.NewAuthService(
				&fakeUserRepo{err: tt.repoErr},
				&fakeSessionRepo{},
				&fakeTokenGen{token: "access-token"},
				&fakeUserAgentParser{},
				discardLogger,
			)
			if err != nil {
				t.Fatalf("NewAuthService returned error: %v", err)
			}

			_, err = service.Register(context.Background(), &models.User{Email: "a@example.com"}, "password123")
			if !errors.Is(err, tt.wantErr) || !errors.Is(err, apperrors.ErrUserAlreadyExists) {
				t.Fatalf("Register error = %v, want %v", err, tt.wantErr)
			}
			if got := apperrors.Reason(err); got != tt.wantReason {
				t.Fatalf("Reason = %q, want %q", got, tt.wantReason)
			}
		})
	}
}

func TestAuthServiceLoginLooksUpOneColumn(t *testing.T) {
	hash, err := utils.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	email, mobileNo := " Ravi@Example.com ", " 9876543210 "

	tests := []struct {
		name     string
		email    *string
		mobileNo *string
		want     string
	}{
		{name: "an email is matched on the email column, lowercased", email: &email, want: "email:ravi@example.com"},
		{name: "a mobile number is matched on the mobile column", mobileNo: &mobileNo, want: "mobile:9876543210"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			users := &fakeUserRepo{user: &models.User{ID: 7, Role: models.RolePatient, PasswordHash: hash}}
			service, err := services.NewAuthService(users, &fakeSessionRepo{}, &fakeTokenGen{}, &fakeUserAgentParser{}, discardLogger)
			if err != nil {
				t.Fatalf("NewAuthService returned error: %v", err)
			}

			if _, _, _, err := service.Login(context.Background(), tt.email, tt.mobileNo, "password123", models.SessionClient{}); err != nil {
				t.Fatalf("Login returned error: %v", err)
			}
			if users.lookedUpBy != tt.want {
				t.Fatalf("Login looked up %q, want %q", users.lookedUpBy, tt.want)
			}
		})
	}
}

func TestAuthServiceRegisterPasswordMinimum(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  error
	}{
		{name: "seven characters is too short", password: "abcdefg", wantErr: apperrors.ErrInvalidRegisterInput},
		{name: "eight characters is enough", password: "abcdefgh"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			users := &fakeUserRepo{user: &models.User{ID: 7}}
			service, err := services.NewAuthService(users, &fakeSessionRepo{}, &fakeTokenGen{}, &fakeUserAgentParser{}, discardLogger)
			if err != nil {
				t.Fatalf("NewAuthService returned error: %v", err)
			}

			_, err = service.Register(context.Background(), &models.User{Name: "Ravi", Email: "ravi@example.com", MobileNo: "9876543210", Role: models.RolePatient}, tt.password)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Register error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil && apperrors.Reason(err) != fmt.Sprintf("password must be at least %d characters", constant.MinPasswordLength) {
				t.Fatalf("Register reason = %q", apperrors.Reason(err))
			}
		})
	}
}

func TestAuthServiceLoginRecordsClient(t *testing.T) {
	hash, err := utils.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	email := "ravi@example.com"
	sessions := &fakeSessionRepo{}
	tokens := &fakeTokenGen{token: "access-token"}
	parser := &fakeUserAgentParser{details: models.UserAgentDetails{BrowserName: "Chrome Mobile", OSName: "Android"}}
	service, err := services.NewAuthService(
		&fakeUserRepo{user: &models.User{ID: 7, Role: models.RolePatient, PasswordHash: hash}},
		sessions,
		tokens,
		parser,
		discardLogger,
	)
	if err != nil {
		t.Fatalf("NewAuthService returned error: %v", err)
	}

	_, _, _, err = service.Login(context.Background(), &email, nil, "password123", models.SessionClient{
		IPAddress:      "203.0.113.10",
		UserAgent:      "  Mozilla/5.0 (Linux; Android 14) Chrome/130.0  ",
		AcceptLanguage: "en-IN,en;q=0.9",
		DeviceID:       "device-1",
		DeviceName:     "Pixel 8",
		DevicePlatform: "Android",
		AppVersion:     "1.4.2",
		Screen:         "1080x2400",
		Timezone:       "Asia/Kolkata",
	})
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}

	created := sessions.created
	if created == nil {
		t.Fatalf("Login created no session")
	}
	if created.IPAddress != "203.0.113.10" || created.Language != "en-IN" || created.DevicePlatform != models.DevicePlatformAndroid {
		t.Fatalf("Login session ip %q language %q platform %q, want the client IP, first language and normalised platform", created.IPAddress, created.Language, created.DevicePlatform)
	}
	if created.DeviceID != "device-1" || created.DeviceName != "Pixel 8" || created.AppVersion != "1.4.2" || created.Screen != "1080x2400" || created.Timezone != "Asia/Kolkata" {
		t.Fatalf("Login session device = %+v, want the device headers", created)
	}
	if parser.parsed != "Mozilla/5.0 (Linux; Android 14) Chrome/130.0" || created.Browser.BrowserName != "Chrome Mobile" {
		t.Fatalf("Login parsed %q into %+v, want the trimmed User-Agent parsed", parser.parsed, created.Browser)
	}
	if tokens.sessionID != 41 {
		t.Fatalf("Login issued a token for session %d, want the created session 41", tokens.sessionID)
	}
}

func TestAuthServiceLoginRejectsBadDeviceHeaders(t *testing.T) {
	hash, err := utils.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	email := "ravi@example.com"

	tests := []struct {
		name       string
		client     models.SessionClient
		wantReason string
	}{
		{name: "unknown platform", client: models.SessionClient{DevicePlatform: "windows"}, wantReason: "X-Device-Platform must be one of web, android, ios"},
		{name: "screen not width x height", client: models.SessionClient{Screen: "large"}, wantReason: "X-Device-Screen must be width x height in pixels, such as 1080x2400"},
		{name: "timezone not IANA", client: models.SessionClient{Timezone: "IST"}, wantReason: "X-Device-Timezone must be an IANA time zone, such as Asia/Kolkata"},
		{name: "server local zone", client: models.SessionClient{Timezone: "Local"}, wantReason: "X-Device-Timezone must be an IANA time zone, such as Asia/Kolkata"},
		{name: "device id too long", client: models.SessionClient{DeviceID: strings.Repeat("a", constant.MaxDeviceIDLength+1)}, wantReason: fmt.Sprintf("X-Device-Id must be at most %d characters", constant.MaxDeviceIDLength)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessions := &fakeSessionRepo{}
			service, err := services.NewAuthService(
				&fakeUserRepo{user: &models.User{ID: 7, Role: models.RolePatient, PasswordHash: hash}},
				sessions,
				&fakeTokenGen{token: "access-token"},
				&fakeUserAgentParser{},
				discardLogger,
			)
			if err != nil {
				t.Fatalf("NewAuthService returned error: %v", err)
			}

			_, _, _, err = service.Login(context.Background(), &email, nil, "password123", tt.client)
			if !errors.Is(err, apperrors.ErrInvalidSessionInput) {
				t.Fatalf("Login error = %v, want %v", err, apperrors.ErrInvalidSessionInput)
			}
			if got := apperrors.Reason(err); got != tt.wantReason {
				t.Fatalf("Reason = %q, want %q", got, tt.wantReason)
			}
			if sessions.createCalled {
				t.Fatalf("Login created a session for a bad header")
			}
		})
	}
}

func TestAuthServiceLoginKeepsBrowserHeaders(t *testing.T) {
	hash, err := utils.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	email := "ravi@example.com"
	sessions := &fakeSessionRepo{}
	service, err := services.NewAuthService(
		&fakeUserRepo{user: &models.User{ID: 7, Role: models.RolePatient, PasswordHash: hash}},
		sessions,
		&fakeTokenGen{token: "access-token"},
		&fakeUserAgentParser{},
		discardLogger,
	)
	if err != nil {
		t.Fatalf("NewAuthService returned error: %v", err)
	}

	_, _, _, err = service.Login(context.Background(), &email, nil, "password123", models.SessionClient{
		UserAgent:      strings.Repeat("u", constant.MaxUserAgentLength+50),
		AcceptLanguage: "*",
	})
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}
	if got := len(sessions.created.UserAgent); got != constant.MaxUserAgentLength {
		t.Fatalf("Login stored a User-Agent of %d characters, want it cut to %d", got, constant.MaxUserAgentLength)
	}
	if sessions.created.Language != "" {
		t.Fatalf("Login stored language %q for a wildcard Accept-Language, want none", sessions.created.Language)
	}
}

func TestAuthServiceRefreshDeviceMismatchIsLogged(t *testing.T) {
	var output bytes.Buffer
	jsonLogger, err := logger.New(logger.Config{Level: "info", Format: logger.FormatJSON}, &output)
	if err != nil {
		t.Fatalf("logger.New returned error: %v", err)
	}
	sessions := &fakeSessionRepo{session: &models.Session{ID: 3, UserID: 7, TokenHash: "hash", ExpiresAt: time.Now().Add(time.Hour), DeviceID: "device-1"}}
	service, err := services.NewAuthService(&fakeUserRepo{user: &models.User{ID: 7}}, sessions, &fakeTokenGen{token: "access-token"}, &fakeUserAgentParser{}, jsonLogger)
	if err != nil {
		t.Fatalf("NewAuthService returned error: %v", err)
	}

	if _, _, err := service.Refresh(context.Background(), "refresh-token", models.SessionClient{DeviceID: "device-2"}); err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}

	var warning map[string]any
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("log line %q is not JSON: %v", line, err)
		}
		if record["msg"] == "refresh device mismatch" {
			warning = record
		}
	}
	if warning == nil || warning["level"] != "WARN" || warning["session_id"] != float64(3) || warning["user_id"] != float64(7) {
		t.Fatalf("Refresh logged %s, want a WARN refresh device mismatch with session and user ids", output.String())
	}
	if strings.Contains(output.String(), "device-2") {
		t.Fatalf("Refresh logged the device id: %s", output.String())
	}
	if sessions.rotated == nil {
		t.Fatalf("Refresh did not rotate the session after a device mismatch")
	}
}

func TestAuthServiceRefreshLosesRace(t *testing.T) {
	sessions := &fakeSessionRepo{
		session:   &models.Session{ID: 3, UserID: 7, TokenHash: "hash", ExpiresAt: time.Now().Add(time.Hour)},
		rotateErr: apperrors.ErrSessionNotFound,
	}
	service, err := services.NewAuthService(&fakeUserRepo{user: &models.User{ID: 7}}, sessions, &fakeTokenGen{token: "access-token"}, &fakeUserAgentParser{}, discardLogger)
	if err != nil {
		t.Fatalf("NewAuthService returned error: %v", err)
	}

	_, _, err = service.Refresh(context.Background(), "refresh-token", models.SessionClient{})
	if !errors.Is(err, apperrors.ErrSessionNotFound) {
		t.Fatalf("Refresh error = %v, want %v", err, apperrors.ErrSessionNotFound)
	}
}
