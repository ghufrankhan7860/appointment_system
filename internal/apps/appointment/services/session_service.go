package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
)

// SessionService lets users see and end their own sessions (one per device
// login) and admins do the same for any user. Revoking a session stops its
// refresh token at once; an access token already issued for it keeps working
// until it expires (constant.AccessTokenTTL).
type SessionService struct {
	sessions SessionRepository
	users    UserRepository
	now      func() time.Time
	logger   Logger
}

func NewSessionService(
	sessions SessionRepository,
	users UserRepository,
	now func() time.Time,
	logger Logger,
) (*SessionService, error) {
	if sessions == nil {
		return nil, apperrors.ErrNilSessionRepository
	}
	if users == nil {
		return nil, apperrors.ErrNilUserRepository
	}
	if now == nil {
		return nil, apperrors.ErrNilClock
	}
	if logger == nil {
		return nil, apperrors.ErrNilLogger
	}

	return &SessionService{
		sessions: sessions,
		users:    users,
		now:      now,
		logger:   logger,
	}, nil
}

// List returns the acting user's active sessions, most recently used first.
func (ss *SessionService) List(
	ctx context.Context,
	actingUserID uint64,
) ([]*models.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list sessions context: %w", err)
	}
	if actingUserID == 0 {
		return nil, fmt.Errorf("list sessions without acting user: %w", apperrors.ErrInvalidSessionInput)
	}

	sessions, err := ss.sessions.ListActiveByUser(ctx, actingUserID, ss.now())
	if err != nil {
		return nil, fmt.Errorf("list active sessions: %w", err)
	}

	return sessions, nil
}

// Revoke ends one of the acting user's sessions, including the current one. A
// session of another user is ErrSessionNotFound.
func (ss *SessionService) Revoke(
	ctx context.Context,
	actingUserID uint64,
	sessionID uint64,
) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("revoke session context: %w", err)
	}
	if actingUserID == 0 || sessionID == 0 {
		return fmt.Errorf("revoke session with missing ids: %w", apperrors.ErrInvalidSessionInput)
	}

	if err := ss.sessions.RevokeByUserAndID(ctx, actingUserID, sessionID); err != nil {
		if errors.Is(err, apperrors.ErrSessionNotFound) {
			return err
		}
		return fmt.Errorf("revoke session: %w", err)
	}
	ss.logger.InfoContext(ctx, "session revoked", "session_id", sessionID, "user_id", actingUserID)

	return nil
}

// RevokeOthers ends every session of the acting user except the current one and
// returns how many it ended.
func (ss *SessionService) RevokeOthers(
	ctx context.Context,
	actingUserID uint64,
	currentSessionID uint64,
) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("revoke other sessions context: %w", err)
	}
	if actingUserID == 0 || currentSessionID == 0 {
		return 0, fmt.Errorf("revoke other sessions with missing ids: %w", apperrors.ErrInvalidSessionInput)
	}

	revoked, err := ss.sessions.RevokeActiveByUser(ctx, actingUserID, currentSessionID)
	if err != nil {
		return 0, fmt.Errorf("revoke other sessions: %w", err)
	}
	ss.logger.InfoContext(ctx, "sessions revoked", "user_id", actingUserID, "kept_session_id", currentSessionID, "count", revoked)

	return revoked, nil
}

// ListForUser returns a page of a user's sessions in every status, for admins.
// The status is worked out from the session's revoked and expiry times.
func (ss *SessionService) ListForUser(
	ctx context.Context,
	userID uint64,
	limit int,
	offset int,
) ([]*models.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list user sessions context: %w", err)
	}
	if err := ss.checkUser(ctx, userID); err != nil {
		return nil, err
	}
	limit, offset = clampPagination(limit, offset)

	sessions, err := ss.sessions.ListByUser(ctx, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list user sessions: %w", err)
	}

	return sessions, nil
}

// RevokeForUser ends one session of a user, for admins.
func (ss *SessionService) RevokeForUser(
	ctx context.Context,
	userID uint64,
	sessionID uint64,
) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("revoke user session context: %w", err)
	}
	if sessionID == 0 {
		return fmt.Errorf("revoke user session without id: %w", apperrors.ErrInvalidSessionInput)
	}
	if err := ss.checkUser(ctx, userID); err != nil {
		return err
	}

	if err := ss.sessions.RevokeByUserAndID(ctx, userID, sessionID); err != nil {
		if errors.Is(err, apperrors.ErrSessionNotFound) {
			return err
		}
		return fmt.Errorf("revoke user session: %w", err)
	}
	ss.logger.InfoContext(ctx, "session revoked", "session_id", sessionID, "user_id", userID)

	return nil
}

// RevokeAllForUser ends every session of a user, for admins, and returns how
// many it ended.
func (ss *SessionService) RevokeAllForUser(ctx context.Context, userID uint64) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("revoke all user sessions context: %w", err)
	}
	if err := ss.checkUser(ctx, userID); err != nil {
		return 0, err
	}

	revoked, err := ss.sessions.RevokeActiveByUser(ctx, userID, 0)
	if err != nil {
		return 0, fmt.Errorf("revoke all user sessions: %w", err)
	}
	ss.logger.InfoContext(ctx, "sessions revoked", "user_id", userID, "count", revoked)

	return revoked, nil
}

// Purge deletes, in batches of limit, the sessions that expired or were revoked
// more than constant.SessionRetention ago, and returns how many it deleted.
func (ss *SessionService) Purge(ctx context.Context, limit int) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("purge sessions context: %w", err)
	}
	if limit <= 0 {
		limit = constant.SessionPurgeBatchSize
	}

	before := ss.now().Add(-constant.SessionRetention)
	purged := 0
	for {
		deleted, err := ss.sessions.DeleteEndedBefore(ctx, before, limit)
		if err != nil {
			return purged, fmt.Errorf("delete ended sessions: %w", err)
		}
		purged += int(deleted)
		if deleted < int64(limit) {
			break
		}
	}
	if purged > 0 {
		ss.logger.InfoContext(ctx, "sessions purged", "count", purged)
	}

	return purged, nil
}

// Status reports whether a session is active, revoked or expired at now.
func (ss *SessionService) Status(session *models.Session) models.SessionStatus {
	switch {
	case session.RevokedAt != nil:
		return models.SessionStatusRevoked
	case !ss.now().Before(session.ExpiresAt):
		return models.SessionStatusExpired
	default:
		return models.SessionStatusActive
	}
}

func (ss *SessionService) checkUser(ctx context.Context, userID uint64) error {
	if userID == 0 {
		return fmt.Errorf("session action without user: %w", apperrors.ErrInvalidSessionInput)
	}
	if _, err := ss.users.GetByID(ctx, userID); err != nil {
		if errors.Is(err, apperrors.ErrUserNotFound) {
			return err
		}
		return fmt.Errorf("get session user: %w", err)
	}

	return nil
}
