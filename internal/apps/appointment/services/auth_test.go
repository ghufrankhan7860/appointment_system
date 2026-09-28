package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
)

type fakeUserRepo struct {
	user *models.User
	err  error
}

func (f *fakeUserRepo) Create(ctx context.Context, user *models.User) (*models.User, error) {
	return f.user, f.err
}

func (f *fakeUserRepo) FindByLogin(ctx context.Context, identifier string) (*models.User, error) {
	return f.user, f.err
}

func (f *fakeUserRepo) GetByID(ctx context.Context, id uint64) (*models.User, error) {
	return f.user, f.err
}

type fakeSessionRepo struct {
	session      *models.Session
	getErr       error
	createErr    error
	revokeErr    error
	createCalled bool
	revokeCalled bool
}

func (f *fakeSessionRepo) Create(ctx context.Context, session *models.Session) error {
	f.createCalled = true
	return f.createErr
}

func (f *fakeSessionRepo) GetByTokenHash(ctx context.Context, tokenHash string) (*models.Session, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.session, nil
}

func (f *fakeSessionRepo) RevokeByID(ctx context.Context, id uint64) error {
	f.revokeCalled = true
	return f.revokeErr
}

type fakeTokenGen struct {
	token string
	err   error
}

func (f *fakeTokenGen) Generate(user models.User) (string, error) {
	return f.token, f.err
}

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
			service, err := NewAuthService(
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
	service, err := NewAuthService(
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
	service, err := NewAuthService(
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
	if _, err := NewAuthService(nil, &fakeSessionRepo{}, &fakeTokenGen{}, tx); !errors.Is(err, apperrors.ErrNilUserRepository) {
		t.Fatalf("nil users error = %v", err)
	}
	if _, err := NewAuthService(&fakeUserRepo{}, nil, &fakeTokenGen{}, tx); !errors.Is(err, apperrors.ErrNilSessionRepository) {
		t.Fatalf("nil sessions error = %v", err)
	}
	if _, err := NewAuthService(&fakeUserRepo{}, &fakeSessionRepo{}, nil, tx); !errors.Is(err, apperrors.ErrNilTokenGenerator) {
		t.Fatalf("nil tokens error = %v", err)
	}
	if _, err := NewAuthService(&fakeUserRepo{}, &fakeSessionRepo{}, &fakeTokenGen{}, nil); !errors.Is(err, apperrors.ErrNilTransactionManager) {
		t.Fatalf("nil tx error = %v", err)
	}
}