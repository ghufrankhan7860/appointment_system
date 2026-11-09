package repositories

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type PatientInviteAttemptRepository struct {
	db *gorm.DB
}

func NewPatientInviteAttemptRepository(db *gorm.DB) (*PatientInviteAttemptRepository, error) {
	if db == nil {
		return nil, apperrors.ErrNilDB
	}
	return &PatientInviteAttemptRepository{db: db}, nil
}

type patientInviteAttemptRecord struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement;column:id"`
	UserID    uint64    `gorm:"column:user_id"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (patientInviteAttemptRecord) TableName() string {
	return "patient_invite_attempts"
}

func (repo *PatientInviteAttemptRepository) Create(ctx context.Context, userID uint64, at time.Time) error {
	record := patientInviteAttemptRecord{
		UserID:    userID,
		CreatedAt: at.UTC(),
	}

	if err := dbFromContext(ctx, repo.db).Create(&record).Error; err != nil {
		return fmt.Errorf("create patient invite attempt record: %w", err)
	}

	return nil
}

func (repo *PatientInviteAttemptRepository) CountSince(ctx context.Context, userID uint64, since time.Time) (int64, error) {
	var count int64

	if err := dbFromContext(ctx, repo.db).
		Model(&patientInviteAttemptRecord{}).
		Where("user_id = ? AND created_at > ?", userID, since.UTC()).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count patient invite attempts: %w", err)
	}

	return count, nil
}

func (repo *PatientInviteAttemptRepository) DeleteBefore(ctx context.Context, userID uint64, before time.Time) error {
	if err := dbFromContext(ctx, repo.db).
		Where("user_id = ? AND created_at <= ?", userID, before.UTC()).
		Delete(&patientInviteAttemptRecord{}).Error; err != nil {
		return fmt.Errorf("delete patient invite attempts: %w", err)
	}

	return nil
}
