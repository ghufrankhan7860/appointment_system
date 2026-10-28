package services_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
	"appointment_system/internal/apps/appointment/utils"
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
				&fakeTxManager{},
			)
			if err != nil {
				t.Fatalf("NewAuthService returned error: %v", err)
			}

			_, _, err = service.Refresh(context.Background(), "refresh-token")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Refresh error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestAuthServiceRefreshActiveSessionRotatesAtomically(t *testing.T) {
	tx := &fakeTxManager{}
	sessions := &fakeSessionRepo{
		session: &models.Session{
			ID:        3,
			UserID:    7,
			TokenHash: "hash",
			ExpiresAt: time.Now().Add(time.Hour),
			RevokedAt: nil,
		},
	}
	service, err := services.NewAuthService(
		&fakeUserRepo{user: &models.User{ID: 7}},
		sessions,
		&fakeTokenGen{token: "access-token"},
		tx,
	)
	if err != nil {
		t.Fatalf("NewAuthService returned error: %v", err)
	}

	accessToken, nextRefreshToken, err := service.Refresh(context.Background(), "refresh-token")
	if err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}
	if accessToken == "" || nextRefreshToken == "" {
		t.Fatalf("Refresh returned empty tokens: %q, %q", accessToken, nextRefreshToken)
	}
	if !tx.called {
		t.Fatalf("Refresh did not run rotation in a transaction")
	}
	if !sessions.createCalled {
		t.Fatalf("Refresh did not create the rotated session")
	}
	if !sessions.revokeCalled {
		t.Fatalf("Refresh did not revoke the consumed session")
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
		&fakeTxManager{},
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
	tx := &fakeTxManager{}
	if _, err := services.NewAuthService(nil, &fakeSessionRepo{}, &fakeTokenGen{}, tx); !errors.Is(err, apperrors.ErrNilUserRepository) {
		t.Fatalf("nil users error = %v", err)
	}
	if _, err := services.NewAuthService(&fakeUserRepo{}, nil, &fakeTokenGen{}, tx); !errors.Is(err, apperrors.ErrNilSessionRepository) {
		t.Fatalf("nil sessions error = %v", err)
	}
	if _, err := services.NewAuthService(&fakeUserRepo{}, &fakeSessionRepo{}, nil, tx); !errors.Is(err, apperrors.ErrNilTokenGenerator) {
		t.Fatalf("nil tokens error = %v", err)
	}
	if _, err := services.NewAuthService(&fakeUserRepo{}, &fakeSessionRepo{}, &fakeTokenGen{}, nil); !errors.Is(err, apperrors.ErrNilTransactionManager) {
		t.Fatalf("nil tx error = %v", err)
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
				&fakeTxManager{},
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
			service, err := services.NewAuthService(users, &fakeSessionRepo{}, &fakeTokenGen{}, &fakeTxManager{})
			if err != nil {
				t.Fatalf("NewAuthService returned error: %v", err)
			}

			if _, _, _, err := service.Login(context.Background(), tt.email, tt.mobileNo, "password123"); err != nil {
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
			service, err := services.NewAuthService(users, &fakeSessionRepo{}, &fakeTokenGen{}, &fakeTxManager{})
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
