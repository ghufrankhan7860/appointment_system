package services_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
)

var sessionNow = time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)

func newTestSessionService(t *testing.T, sessions *fakeSessionRepo, users *fakeUserRepo) *services.SessionService {
	t.Helper()

	service, err := services.NewSessionService(sessions, users, func() time.Time { return sessionNow }, discardLogger)
	if err != nil {
		t.Fatalf("NewSessionService returned error: %v", err)
	}

	return service
}

func TestNewSessionServiceValidation(t *testing.T) {
	tests := []struct {
		name     string
		sessions services.SessionRepository
		users    services.UserRepository
		now      func() time.Time
		wantErr  error
	}{
		{name: "nil session repository", users: &fakeUserRepo{}, now: time.Now, wantErr: apperrors.ErrNilSessionRepository},
		{name: "nil user repository", sessions: &fakeSessionRepo{}, now: time.Now, wantErr: apperrors.ErrNilUserRepository},
		{name: "nil clock", sessions: &fakeSessionRepo{}, users: &fakeUserRepo{}, wantErr: apperrors.ErrNilClock},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := services.NewSessionService(tt.sessions, tt.users, tt.now, discardLogger); !errors.Is(err, tt.wantErr) {
				t.Fatalf("NewSessionService error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestSessionServiceList(t *testing.T) {
	sessions := &fakeSessionRepo{sessions: []*models.Session{{ID: 21, UserID: 7}, {ID: 22, UserID: 7}}}
	service := newTestSessionService(t, sessions, &fakeUserRepo{})

	listed, err := service.List(context.Background(), 7)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(listed) != 2 || !sessions.listedNow.Equal(sessionNow) {
		t.Fatalf("List = %d sessions active at %v, want 2 active at %v", len(listed), sessions.listedNow, sessionNow)
	}
}

func TestSessionServiceRevoke(t *testing.T) {
	tests := []struct {
		name      string
		sessionID uint64
		repoErr   error
		wantErr   error
	}{
		{name: "own session is revoked", sessionID: 22},
		{name: "another user's session is not found", sessionID: 99, repoErr: apperrors.ErrSessionNotFound, wantErr: apperrors.ErrSessionNotFound},
		{name: "missing session id", sessionID: 0, wantErr: apperrors.ErrInvalidSessionInput},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessions := &fakeSessionRepo{revokeErr: tt.repoErr}
			service := newTestSessionService(t, sessions, &fakeUserRepo{})

			err := service.Revoke(context.Background(), 7, tt.sessionID)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Revoke error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && (sessions.revokedUserID != 7 || sessions.revokedID != tt.sessionID) {
				t.Fatalf("Revoke revoked session %d of user %d, want %d of 7", sessions.revokedID, sessions.revokedUserID, tt.sessionID)
			}
		})
	}
}

func TestSessionServiceRevokeOthers(t *testing.T) {
	tests := []struct {
		name             string
		currentSessionID uint64
		wantRevoked      int64
		wantErr          error
	}{
		{name: "keeps the current session", currentSessionID: 21, wantRevoked: 2},
		{name: "unknown current session", currentSessionID: 0, wantErr: apperrors.ErrInvalidSessionInput},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessions := &fakeSessionRepo{revokedCount: 2}
			service := newTestSessionService(t, sessions, &fakeUserRepo{})

			revoked, err := service.RevokeOthers(context.Background(), 7, tt.currentSessionID)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("RevokeOthers error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			if revoked != tt.wantRevoked || sessions.revokedUserID != 7 || sessions.exceptID != tt.currentSessionID {
				t.Fatalf("RevokeOthers revoked %d of user %d except %d, want %d of 7 except %d", revoked, sessions.revokedUserID, sessions.exceptID, tt.wantRevoked, tt.currentSessionID)
			}
		})
	}
}

func TestSessionServiceListForUser(t *testing.T) {
	tests := []struct {
		name       string
		users      *fakeUserRepo
		limit      int
		offset     int
		wantLimit  int
		wantOffset int
		wantErr    error
	}{
		{name: "default page", users: &fakeUserRepo{user: &models.User{ID: 7}}, wantLimit: constant.DefaultListLimit},
		{name: "page capped", users: &fakeUserRepo{user: &models.User{ID: 7}}, limit: 1000, offset: 20, wantLimit: constant.MaxListLimit, wantOffset: 20},
		{name: "unknown user", users: &fakeUserRepo{err: apperrors.ErrUserNotFound}, wantErr: apperrors.ErrUserNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessions := &fakeSessionRepo{}
			service := newTestSessionService(t, sessions, tt.users)

			_, err := service.ListForUser(context.Background(), 7, tt.limit, tt.offset)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ListForUser error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && (sessions.listedLimit != tt.wantLimit || sessions.listedOffset != tt.wantOffset) {
				t.Fatalf("ListForUser paged %d/%d, want %d/%d", sessions.listedLimit, sessions.listedOffset, tt.wantLimit, tt.wantOffset)
			}
		})
	}
}

func TestSessionServiceRevokeAllForUser(t *testing.T) {
	sessions := &fakeSessionRepo{revokedCount: 3}
	service := newTestSessionService(t, sessions, &fakeUserRepo{user: &models.User{ID: 7}})

	revoked, err := service.RevokeAllForUser(context.Background(), 7)
	if err != nil {
		t.Fatalf("RevokeAllForUser returned error: %v", err)
	}
	if revoked != 3 || sessions.exceptID != 0 {
		t.Fatalf("RevokeAllForUser revoked %d keeping %d, want 3 keeping none", revoked, sessions.exceptID)
	}
}

func TestSessionServiceStatus(t *testing.T) {
	revokedAt := sessionNow.Add(-time.Hour)

	tests := []struct {
		name    string
		session *models.Session
		want    models.SessionStatus
	}{
		{name: "active", session: &models.Session{ExpiresAt: sessionNow.Add(time.Hour)}, want: models.SessionStatusActive},
		{name: "expired", session: &models.Session{ExpiresAt: sessionNow}, want: models.SessionStatusExpired},
		{name: "revoked wins over expiry", session: &models.Session{ExpiresAt: sessionNow.Add(-time.Hour), RevokedAt: &revokedAt}, want: models.SessionStatusRevoked},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := newTestSessionService(t, &fakeSessionRepo{}, &fakeUserRepo{})
			if got := service.Status(tt.session); got != tt.want {
				t.Fatalf("Status = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSessionServicePurge(t *testing.T) {
	sessions := &fakeSessionRepo{deleteResults: []int64{2, 2, 1}}
	service := newTestSessionService(t, sessions, &fakeUserRepo{})

	purged, err := service.Purge(context.Background(), 2)
	if err != nil {
		t.Fatalf("Purge returned error: %v", err)
	}
	if purged != 5 || sessions.deleteCalls != 3 {
		t.Fatalf("Purge deleted %d in %d batches, want 5 in 3", purged, sessions.deleteCalls)
	}
	if want := sessionNow.Add(-constant.SessionRetention); !sessions.deleteBefore.Equal(want) {
		t.Fatalf("Purge cutoff = %v, want %v", sessions.deleteBefore, want)
	}
}
