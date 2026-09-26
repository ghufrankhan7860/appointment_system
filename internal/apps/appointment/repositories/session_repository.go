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
	ID        uint64     `gorm:"primaryKey"`
	UserID    uint64     `gorm:"index"`
	TokenHash string     `gorm:"uniqueIndex"`
	ExpiresAt time.Time  `gorm:"column:expires_at"`
	RevokedAt *time.Time `gorm:"column:revoked_at"`
	CreatedAt time.Time  `gorm:"column:created_at"`
	UpdatedAt time.Time  `gorm:"column:updated_at"`
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
		UserID:    session.UserID,
		TokenHash: session.TokenHash,
		ExpiresAt: session.ExpiresAt,
		RevokedAt: nil,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := sr.db.WithContext(ctx).Create(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return apperrors.ErrTokenAlreadyExists
		}

		return fmt.Errorf("error saving refresh token: %w", err)
	}

	session.ID = record.ID
	session.ExpiresAt = record.ExpiresAt
	session.RevokedAt = record.RevokedAt
	session.CreatedAt = record.CreatedAt
	session.UpdatedAt = record.UpdatedAt

	return nil
}

func (sr *SessionRepository) GetByTokenHash(
	ctx context.Context,
	tokenHash string,
) (*models.Session, error) {
	var record sessionRecord

	if err := sr.db.WithContext(ctx).
		Where("token_hash = ?", tokenHash).
		First(&record).Error; err != nil {

		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrSessionNotFound
		}

		return nil, fmt.Errorf("error fetching session by token hash: %w", err)
	}

	session := &models.Session{
		ID:        record.ID,
		UserID:    record.UserID,
		TokenHash: record.TokenHash,
		ExpiresAt: record.ExpiresAt,
		RevokedAt: record.RevokedAt,
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
	}

	return session, nil
}

func (sr *SessionRepository) RevokeByID(
	ctx context.Context,
	id uint64,
) error {
	now := time.Now().UTC()
	result := sr.db.WithContext(ctx).
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

func (sr *SessionRepository) Rotate(
	ctx context.Context,
	next *models.Session,
	currentID uint64,
) error {
	if next == nil {
		return apperrors.ErrNilSessionRequest
	}
	if next.ExpiresAt.IsZero() {
		return apperrors.ErrSessionExpiryMissing
	}

	return sr.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()

		nextRecord := sessionRecord{
			UserID:    next.UserID,
			TokenHash: next.TokenHash,
			ExpiresAt: next.ExpiresAt,
			RevokedAt: nil,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := tx.Create(&nextRecord).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return apperrors.ErrTokenAlreadyExists
			}
			return fmt.Errorf("error saving rotated refresh token: %w", err)
		}

		revoke := tx.Model(&sessionRecord{}).
			Where("id = ? AND revoked_at IS NULL", currentID).
			Updates(map[string]interface{}{
				"revoked_at": now,
				"updated_at": now,
			})
		if revoke.Error != nil {
			return fmt.Errorf("error revoking rotated session: %w", revoke.Error)
		}
		if revoke.RowsAffected == 0 {
			return apperrors.ErrSessionNotFound
		}

		next.ID = nextRecord.ID
		next.ExpiresAt = nextRecord.ExpiresAt
		next.RevokedAt = nextRecord.RevokedAt
		next.CreatedAt = nextRecord.CreatedAt
		next.UpdatedAt = nextRecord.UpdatedAt

		return nil
	})
}
