package repositories

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CallGuestTokenRepository struct {
	db *gorm.DB
}

func NewCallGuestTokenRepository(db *gorm.DB) (*CallGuestTokenRepository, error) {
	if db == nil {
		return nil, apperrors.ErrNilDB
	}
	return &CallGuestTokenRepository{db: db}, nil
}

type callGuestTokenRecord struct {
	ID              uint64     `gorm:"primaryKey;autoIncrement;column:id"`
	AppointmentID   uint64     `gorm:"column:appointment_id"`
	CreatedByUserID uint64     `gorm:"column:created_by_user_id"`
	TokenHash       string     `gorm:"column:token_hash"`
	DisplayName     string     `gorm:"column:display_name"`
	ExpiresAt       time.Time  `gorm:"column:expires_at"`
	MaxUses         int        `gorm:"column:max_uses"`
	UseCount        int        `gorm:"column:use_count"`
	RevokedAt       *time.Time `gorm:"column:revoked_at"`
	CreatedAt       time.Time  `gorm:"column:created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at"`
}

func (callGuestTokenRecord) TableName() string {
	return "call_guest_tokens"
}

func (repo *CallGuestTokenRepository) Create(ctx context.Context, guestToken *models.CallGuestToken) (*models.CallGuestToken, error) {
	if guestToken == nil {
		return nil, apperrors.ErrNilCallRequest
	}

	now := time.Now().UTC()
	record := callGuestTokenRecord{
		AppointmentID:   guestToken.AppointmentID,
		CreatedByUserID: guestToken.CreatedByUserID,
		TokenHash:       guestToken.TokenHash,
		DisplayName:     guestToken.DisplayName,
		ExpiresAt:       guestToken.ExpiresAt.UTC(),
		MaxUses:         guestToken.MaxUses,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := dbFromContext(ctx, repo.db).Create(&record).Error; err != nil {
		return nil, fmt.Errorf("create call guest token record: %w", err)
	}

	return toCallGuestTokenModel(record), nil
}

func (repo *CallGuestTokenRepository) GetByTokenHashForUpdate(ctx context.Context, tokenHash string) (*models.CallGuestToken, error) {
	var record callGuestTokenRecord

	if err := dbFromContext(ctx, repo.db).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("token_hash = ?", tokenHash).
		First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrCallGuestLinkNotFound
		}
		return nil, fmt.Errorf("get call guest token by hash: %w", err)
	}

	return toCallGuestTokenModel(record), nil
}

func (repo *CallGuestTokenRepository) IncrementUse(ctx context.Context, id uint64) error {
	if err := dbFromContext(ctx, repo.db).
		Model(&callGuestTokenRecord{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"use_count":  gorm.Expr("use_count + 1"),
			"updated_at": time.Now().UTC(),
		}).Error; err != nil {
		return fmt.Errorf("increment call guest token use: %w", err)
	}

	return nil
}

func (repo *CallGuestTokenRepository) Revoke(ctx context.Context, appointmentID uint64, id uint64, revokedAt time.Time) error {
	result := dbFromContext(ctx, repo.db).
		Model(&callGuestTokenRecord{}).
		Where("id = ? AND appointment_id = ? AND revoked_at IS NULL", id, appointmentID).
		Updates(map[string]interface{}{
			"revoked_at": revokedAt.UTC(),
			"updated_at": time.Now().UTC(),
		})
	if result.Error != nil {
		return fmt.Errorf("revoke call guest token: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrCallGuestLinkNotFound
	}

	return nil
}

func toCallGuestTokenModel(record callGuestTokenRecord) *models.CallGuestToken {
	return &models.CallGuestToken{
		ID:              record.ID,
		AppointmentID:   record.AppointmentID,
		CreatedByUserID: record.CreatedByUserID,
		TokenHash:       record.TokenHash,
		DisplayName:     record.DisplayName,
		ExpiresAt:       record.ExpiresAt,
		MaxUses:         record.MaxUses,
		UseCount:        record.UseCount,
		RevokedAt:       record.RevokedAt,
		CreatedAt:       record.CreatedAt,
		UpdatedAt:       record.UpdatedAt,
	}
}
