package repositories

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type PatientPassAttemptRepository struct {
	db *gorm.DB
}

func NewPatientPassAttemptRepository(db *gorm.DB) (*PatientPassAttemptRepository, error) {
	if db == nil {
		return nil, apperrors.ErrNilDB
	}
	return &PatientPassAttemptRepository{db: db}, nil
}

type patientPassAttemptRecord struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement;column:id"`
	UserID    uint64    `gorm:"column:user_id"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (patientPassAttemptRecord) TableName() string {
	return "patient_pass_attempts"
}

func (repo *PatientPassAttemptRepository) Create(ctx context.Context, userID uint64, at time.Time) error {
	record := patientPassAttemptRecord{
		UserID:    userID,
		CreatedAt: at.UTC(),
	}

	if err := dbFromContext(ctx, repo.db).Create(&record).Error; err != nil {
		return fmt.Errorf("create patient pass attempt record: %w", err)
	}

	return nil
}

func (repo *PatientPassAttemptRepository) CountSince(ctx context.Context, userID uint64, since time.Time) (int64, error) {
	var count int64

	if err := dbFromContext(ctx, repo.db).
		Model(&patientPassAttemptRecord{}).
		Where("user_id = ? AND created_at > ?", userID, since.UTC()).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count patient pass attempts: %w", err)
	}

	return count, nil
}

func (repo *PatientPassAttemptRepository) DeleteBefore(ctx context.Context, userID uint64, before time.Time) error {
	if err := dbFromContext(ctx, repo.db).
		Where("user_id = ? AND created_at <= ?", userID, before.UTC()).
		Delete(&patientPassAttemptRecord{}).Error; err != nil {
		return fmt.Errorf("delete patient pass attempts: %w", err)
	}

	return nil
}
