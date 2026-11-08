package repositories

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type SessionRepository struct {
	db *gorm.DB
}

func NewSessionRepository(db *gorm.DB) (*SessionRepository, error) {
	if db == nil {
		return nil, apperrors.ErrNilDB
	}

	return &SessionRepository{db: db}, nil
}

type sessionRecord struct {
	ID             uint64     `gorm:"primaryKey"`
	UserID         uint64     `gorm:"index"`
	TokenHash      string     `gorm:"uniqueIndex"`
	ExpiresAt      time.Time  `gorm:"column:expires_at"`
	RevokedAt      *time.Time `gorm:"column:revoked_at"`
	IPAddress      *string    `gorm:"column:ip_address"`
	LastSeenIP     *string    `gorm:"column:last_seen_ip"`
	LastSeenAt     time.Time  `gorm:"column:last_seen_at"`
	UserAgent      string     `gorm:"column:user_agent"`
	BrowserName    string     `gorm:"column:browser_name"`
	BrowserVersion string     `gorm:"column:browser_version"`
	OSName         string     `gorm:"column:os_name"`
	OSVersion      string     `gorm:"column:os_version"`
	DeviceBrand    string     `gorm:"column:device_brand"`
	DeviceModel    string     `gorm:"column:device_model"`
	DeviceID       string     `gorm:"column:device_id"`
	DeviceName     string     `gorm:"column:device_name"`
	DevicePlatform string     `gorm:"column:device_platform"`
	AppVersion     string     `gorm:"column:app_version"`
	Screen         string     `gorm:"column:screen"`
	Timezone       string     `gorm:"column:timezone"`
	Language       string     `gorm:"column:language"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at"`
}

func (sessionRecord) TableName() string {
	return "sessions"
}

func (sr *SessionRepository) Create(
	ctx context.Context,
	session *models.Session,
) error {
	if session == nil {
		return apperrors.ErrNilSessionRequest
	}
	if session.ExpiresAt.IsZero() {
		return apperrors.ErrSessionExpiryMissing
	}

	now := time.Now().UTC()

	record := sessionRecord{
		UserID:         session.UserID,
		TokenHash:      session.TokenHash,
		ExpiresAt:      session.ExpiresAt,
		RevokedAt:      nil,
		IPAddress:      inetValue(session.IPAddress),
		LastSeenIP:     inetValue(session.IPAddress),
		LastSeenAt:     now,
		UserAgent:      session.UserAgent,
		BrowserName:    session.Browser.BrowserName,
		BrowserVersion: session.Browser.BrowserVersion,
		OSName:         session.Browser.OSName,
		OSVersion:      session.Browser.OSVersion,
		DeviceBrand:    session.Browser.DeviceBrand,
		DeviceModel:    session.Browser.DeviceModel,
		DeviceID:       session.DeviceID,
		DeviceName:     session.DeviceName,
		DevicePlatform: string(session.DevicePlatform),
		AppVersion:     session.AppVersion,
		Screen:         session.Screen,
		Timezone:       session.Timezone,
		Language:       session.Language,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := dbFromContext(ctx, sr.db).Create(&record).Error; err != nil {
		if tokenHashUnique.ViolatedBy(err) {
			return apperrors.ErrTokenAlreadyExists
		}

		return fmt.Errorf("error saving refresh token: %w", err)
	}

	*session = *toSessionModel(record)

	return nil
}

func (sr *SessionRepository) GetByTokenHash(
	ctx context.Context,
	tokenHash string,
) (*models.Session, error) {
	var record sessionRecord

	if err := dbFromContext(ctx, sr.db).
		Where("token_hash = ?", tokenHash).
		First(&record).Error; err != nil {

		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrSessionNotFound
		}

		return nil, fmt.Errorf("error fetching session by token hash: %w", err)
	}

	return toSessionModel(record), nil
}

func (sr *SessionRepository) RevokeByID(
	ctx context.Context,
	id uint64,
) error {
	now := time.Now().UTC()
	result := dbFromContext(ctx, sr.db).
		Model(&sessionRecord{}).
		Where("id = ? AND revoked_at IS NULL", id).
		Updates(map[string]interface{}{
			"revoked_at": now,
			"updated_at": now,
		})

	if result.Error != nil {
		return fmt.Errorf("error revoking session: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return apperrors.ErrSessionNotFound
	}

	return nil
}

// Rotate gives a session the token hash, expiry and client details in session.
// It matches only the row still holding oldTokenHash and not revoked, so two
// refreshes racing with one token cannot both win; the loser gets
// ErrSessionNotFound. The login IP and device ID are never changed.
func (sr *SessionRepository) Rotate(
	ctx context.Context,
	id uint64,
	oldTokenHash string,
	session *models.Session,
) error {
	if session == nil {
		return apperrors.ErrNilSessionRequest
	}

	now := time.Now().UTC()
	result := dbFromContext(ctx, sr.db).
		Model(&sessionRecord{}).
		Where("id = ? AND token_hash = ? AND revoked_at IS NULL", id, oldTokenHash).
		Updates(map[string]interface{}{
			"token_hash":      session.TokenHash,
			"expires_at":      session.ExpiresAt,
			"last_seen_ip":    inetValue(session.LastSeenIP),
			"last_seen_at":    now,
			"user_agent":      session.UserAgent,
			"browser_name":    session.Browser.BrowserName,
			"browser_version": session.Browser.BrowserVersion,
			"os_name":         session.Browser.OSName,
			"os_version":      session.Browser.OSVersion,
			"device_brand":    session.Browser.DeviceBrand,
			"device_model":    session.Browser.DeviceModel,
			"app_version":     session.AppVersion,
			"timezone":        session.Timezone,
			"updated_at":      now,
		})
	if result.Error != nil {
		if tokenHashUnique.ViolatedBy(result.Error) {
			return apperrors.ErrTokenAlreadyExists
		}
		return fmt.Errorf("rotate session: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrSessionNotFound
	}

	return nil
}

// ListActiveByUser returns the user's sessions that are neither revoked nor
// expired at now, most recently used first.
func (sr *SessionRepository) ListActiveByUser(
	ctx context.Context,
	userID uint64,
	now time.Time,
) ([]*models.Session, error) {
	var records []sessionRecord

	if err := dbFromContext(ctx, sr.db).
		Where("user_id = ? AND revoked_at IS NULL AND expires_at > ?", userID, now.UTC()).
		Order("last_seen_at DESC, id DESC").
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list active sessions by user: %w", err)
	}

	return toSessionModels(records), nil
}

// ListByUser returns a page of every session the user still has in the table,
// whatever its status, most recently used first.
func (sr *SessionRepository) ListByUser(
	ctx context.Context,
	userID uint64,
	limit int,
	offset int,
) ([]*models.Session, error) {
	var records []sessionRecord

	if err := dbFromContext(ctx, sr.db).
		Where("user_id = ?", userID).
		Order("last_seen_at DESC, id DESC").
		Limit(limit).
		Offset(offset).
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list sessions by user: %w", err)
	}

	return toSessionModels(records), nil
}

// RevokeByUserAndID revokes one active session of the user. A session that is
// someone else's, already revoked or missing is ErrSessionNotFound, so session
// IDs of other users are not revealed.
func (sr *SessionRepository) RevokeByUserAndID(
	ctx context.Context,
	userID uint64,
	id uint64,
) error {
	now := time.Now().UTC()
	result := dbFromContext(ctx, sr.db).
		Model(&sessionRecord{}).
		Where("id = ? AND user_id = ? AND revoked_at IS NULL", id, userID).
		Updates(map[string]interface{}{
			"revoked_at": now,
			"updated_at": now,
		})
	if result.Error != nil {
		return fmt.Errorf("revoke session of user: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrSessionNotFound
	}

	return nil
}

// RevokeActiveByUser revokes every unrevoked session of the user except
// exceptID (0 keeps none) and returns how many it revoked.
func (sr *SessionRepository) RevokeActiveByUser(
	ctx context.Context,
	userID uint64,
	exceptID uint64,
) (int64, error) {
	now := time.Now().UTC()
	result := dbFromContext(ctx, sr.db).
		Model(&sessionRecord{}).
		Where("user_id = ? AND id <> ? AND revoked_at IS NULL", userID, exceptID).
		Updates(map[string]interface{}{
			"revoked_at": now,
			"updated_at": now,
		})
	if result.Error != nil {
		return 0, fmt.Errorf("revoke active sessions by user: %w", result.Error)
	}

	return result.RowsAffected, nil
}

// DeleteEndedBefore deletes up to limit sessions that were revoked or expired
// before the cutoff and returns how many it deleted.
func (sr *SessionRepository) DeleteEndedBefore(
	ctx context.Context,
	before time.Time,
	limit int,
) (int64, error) {
	db := dbFromContext(ctx, sr.db)
	ended := db.Model(&sessionRecord{}).
		Select("id").
		Where("revoked_at < ? OR expires_at < ?", before.UTC(), before.UTC()).
		Order("id ASC").
		Limit(limit)

	result := db.Where("id IN (?)", ended).Delete(&sessionRecord{})
	if result.Error != nil {
		return 0, fmt.Errorf("delete ended sessions: %w", result.Error)
	}

	return result.RowsAffected, nil
}

func toSessionModel(record sessionRecord) *models.Session {
	return &models.Session{
		ID:         record.ID,
		UserID:     record.UserID,
		TokenHash:  record.TokenHash,
		ExpiresAt:  record.ExpiresAt,
		RevokedAt:  record.RevokedAt,
		IPAddress:  inetString(record.IPAddress),
		LastSeenIP: inetString(record.LastSeenIP),
		LastSeenAt: record.LastSeenAt,
		UserAgent:  record.UserAgent,
		Browser: models.UserAgentDetails{
			BrowserName:    record.BrowserName,
			BrowserVersion: record.BrowserVersion,
			OSName:         record.OSName,
			OSVersion:      record.OSVersion,
			DeviceBrand:    record.DeviceBrand,
			DeviceModel:    record.DeviceModel,
		},
		DeviceID:       record.DeviceID,
		DeviceName:     record.DeviceName,
		DevicePlatform: models.DevicePlatform(record.DevicePlatform),
		AppVersion:     record.AppVersion,
		Screen:         record.Screen,
		Timezone:       record.Timezone,
		Language:       record.Language,
		CreatedAt:      record.CreatedAt,
		UpdatedAt:      record.UpdatedAt,
	}
}

func toSessionModels(records []sessionRecord) []*models.Session {
	sessions := make([]*models.Session, 0, len(records))
	for _, record := range records {
		sessions = append(sessions, toSessionModel(record))
	}

	return sessions
}

// inetValue stores an unknown IP as NULL: an empty string is not a valid INET.
func inetValue(ip string) *string {
	if ip == "" {
		return nil
	}

	return &ip
}

func inetString(ip *string) string {
	if ip == nil {
		return ""
	}

	return *ip
}
