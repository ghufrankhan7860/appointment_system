package repositories_test

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/repositories"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestSessionRepositoryLifecycle(t *testing.T) {
	db := openTestDB(t)
	sessions, err := repositories.NewSessionRepository(db)
	if err != nil {
		t.Fatalf("NewSessionRepository returned error: %v", err)
	}
	ctx := context.Background()
	userID := uint64(time.Now().UnixNano()%1_000_000_000) + 1_000_000_000
	otherUserID := userID + 1
	t.Cleanup(func() {
		db.Exec("DELETE FROM sessions WHERE user_id IN (?, ?)", userID, otherUserID)
	})

	newSession := func(userID uint64, expiresAt time.Time) *models.Session {
		t.Helper()
		session := &models.Session{
			UserID:         userID,
			TokenHash:      fmt.Sprintf("hash-%d", time.Now().UnixNano()),
			ExpiresAt:      expiresAt,
			IPAddress:      "203.0.113.10",
			UserAgent:      "Mozilla/5.0",
			Browser:        models.UserAgentDetails{BrowserName: "Chrome", BrowserVersion: "130.0", OSName: "Android", OSVersion: "14"},
			DeviceID:       "device-1",
			DevicePlatform: models.DevicePlatformAndroid,
			AppVersion:     "1.0.0",
			Language:       "en-IN",
		}
		if err := sessions.Create(ctx, session); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		return session
	}

	current := newSession(userID, time.Now().Add(time.Hour))
	if current.ID == 0 || current.LastSeenIP != "203.0.113.10" || current.LastSeenAt.IsZero() {
		t.Fatalf("Create = %+v, want an ID and the login IP as last seen", current)
	}
	stored, err := sessions.GetByTokenHash(ctx, current.TokenHash)
	if err != nil {
		t.Fatalf("GetByTokenHash returned error: %v", err)
	}
	if stored.IPAddress != "203.0.113.10" || stored.Browser.BrowserName != "Chrome" || stored.DevicePlatform != models.DevicePlatformAndroid || stored.Language != "en-IN" {
		t.Fatalf("GetByTokenHash = %+v, want the client details saved", stored)
	}

	t.Run("rotate swaps the token and keeps the login details", func(t *testing.T) {
		oldHash := current.TokenHash
		rotated := *current
		rotated.TokenHash = fmt.Sprintf("hash-%d", time.Now().UnixNano())
		rotated.LastSeenIP = "198.51.100.7"
		rotated.AppVersion = "1.1.0"
		if err := sessions.Rotate(ctx, current.ID, oldHash, &rotated); err != nil {
			t.Fatalf("Rotate returned error: %v", err)
		}
		if err := sessions.Rotate(ctx, current.ID, oldHash, &rotated); !errors.Is(err, apperrors.ErrSessionNotFound) {
			t.Fatalf("Rotate with the old hash error = %v, want %v", err, apperrors.ErrSessionNotFound)
		}

		reloaded, err := sessions.GetByTokenHash(ctx, rotated.TokenHash)
		if err != nil {
			t.Fatalf("GetByTokenHash returned error: %v", err)
		}
		if reloaded.ID != current.ID || reloaded.LastSeenIP != "198.51.100.7" || reloaded.IPAddress != "203.0.113.10" || reloaded.AppVersion != "1.1.0" || reloaded.DeviceID != "device-1" {
			t.Fatalf("rotated session = %+v, want the same row with last seen updated and login details kept", reloaded)
		}
		current = reloaded
	})

	t.Run("revoke checks the owner", func(t *testing.T) {
		other := newSession(userID, time.Now().Add(time.Hour))
		if err := sessions.RevokeByUserAndID(ctx, otherUserID, other.ID); !errors.Is(err, apperrors.ErrSessionNotFound) {
			t.Fatalf("RevokeByUserAndID by another user error = %v, want %v", err, apperrors.ErrSessionNotFound)
		}
		if err := sessions.RevokeByUserAndID(ctx, userID, other.ID); err != nil {
			t.Fatalf("RevokeByUserAndID returned error: %v", err)
		}
		if err := sessions.RevokeByUserAndID(ctx, userID, other.ID); !errors.Is(err, apperrors.ErrSessionNotFound) {
			t.Fatalf("RevokeByUserAndID twice error = %v, want %v", err, apperrors.ErrSessionNotFound)
		}
	})

	t.Run("revoke active keeps one session", func(t *testing.T) {
		newSession(userID, time.Now().Add(time.Hour))
		newSession(userID, time.Now().Add(time.Hour))
		revoked, err := sessions.RevokeActiveByUser(ctx, userID, current.ID)
		if err != nil {
			t.Fatalf("RevokeActiveByUser returned error: %v", err)
		}
		if revoked != 2 {
			t.Fatalf("RevokeActiveByUser revoked %d, want 2", revoked)
		}

		active, err := sessions.ListActiveByUser(ctx, userID, time.Now())
		if err != nil {
			t.Fatalf("ListActiveByUser returned error: %v", err)
		}
		if len(active) != 1 || active[0].ID != current.ID {
			t.Fatalf("ListActiveByUser = %d sessions, want only the kept session %d", len(active), current.ID)
		}

		all, err := sessions.ListByUser(ctx, userID, 100, 0)
		if err != nil {
			t.Fatalf("ListByUser returned error: %v", err)
		}
		if len(all) != 4 {
			t.Fatalf("ListByUser = %d sessions, want all 4 whatever their status", len(all))
		}
	})

	t.Run("delete ended before the cutoff", func(t *testing.T) {
		ended := newSession(otherUserID, time.Now().Add(-200*24*time.Hour))
		recent := newSession(otherUserID, time.Now().Add(-time.Hour))

		if _, err := sessions.DeleteEndedBefore(ctx, time.Now().Add(-90*24*time.Hour), 1000); err != nil {
			t.Fatalf("DeleteEndedBefore returned error: %v", err)
		}
		if _, err := sessions.GetByTokenHash(ctx, ended.TokenHash); !errors.Is(err, apperrors.ErrSessionNotFound) {
			t.Fatalf("session ended 200 days ago error = %v, want it deleted", err)
		}
		if _, err := sessions.GetByTokenHash(ctx, recent.TokenHash); err != nil {
			t.Fatalf("session ended an hour ago error = %v, want it kept", err)
		}
	})
}
