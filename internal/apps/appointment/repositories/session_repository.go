package repositories

import (
	"appointment_system/internal/apps/appointment/constant"
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
	ID        uint64 `gorm:"primaryKey"`
	UserID    uint64 `gorm:"index"`
	TokenHash string `gorm:"uniqueIndex"`
	Expiry    time.Time
	IsRevoked bool
	CreatedAt time.Time
	UpdatedAt time.Time
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

	now := time.Now()

	record := sessionRecord{
		UserID:    session.UserID,
		TokenHash: session.TokenHash,
		Expiry:    now.AddDate(0, 0, constant.REFRESH_TOKEN_EXPIRATION_DAYS),
		IsRevoked: false,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := sr.db.WithContext(ctx).Create(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return apperrors.ErrTokenAlreadyExists
		}

		return fmt.Errorf("error saving refresh token: %w", err)
	}

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
		Expiry:    record.Expiry,
		IsRevoked: record.IsRevoked,
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
	}

	return session, nil
}

func (sr *SessionRepository) RevokeByID(
	ctx context.Context,
	id uint64,
) error {
	result := sr.db.WithContext(ctx).
		Model(&sessionRecord{}).
		Where("id = ? AND is_revoked = ?", id, false).
		Updates(map[string]interface{}{
			"is_revoked": true,
			"updated_at": time.Now(),
		})

	if result.Error != nil {
		return fmt.Errorf("error revoking session: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return apperrors.ErrSessionNotFound
	}

	return nil
}
