package repositories

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type UserPatientHistoryRepository struct {
	db *gorm.DB
}

func NewUserPatientHistoryRepository(db *gorm.DB) (*UserPatientHistoryRepository, error) {
	if db == nil {
		return nil, apperrors.ErrNilDB
	}
	return &UserPatientHistoryRepository{db: db}, nil
}

type userPatientHistoryRecord struct {
	ID              uint64    `gorm:"primaryKey;autoIncrement;column:id"`
	UserID          uint64    `gorm:"column:user_id"`
	PatientID       uint64    `gorm:"column:patient_id"`
	ChangedByUserID uint64    `gorm:"column:changed_by_user_id"`
	FromStatus      string    `gorm:"column:from_status"`
	ToStatus        string    `gorm:"column:to_status"`
	FromAccessLevel string    `gorm:"column:from_access_level"`
	ToAccessLevel   string    `gorm:"column:to_access_level"`
	Reason          string    `gorm:"column:reason"`
	CreatedAt       time.Time `gorm:"column:created_at"`
}

func (userPatientHistoryRecord) TableName() string {
	return "user_patient_history"
}

func (repo *UserPatientHistoryRepository) Create(ctx context.Context, history *models.UserPatientHistory) error {
	if history == nil {
		return apperrors.ErrNilUserPatientRequest
	}

	record := userPatientHistoryRecord{
		UserID:          history.UserID,
		PatientID:       history.PatientID,
		ChangedByUserID: history.ChangedByUserID,
		FromStatus:      string(history.FromStatus),
		ToStatus:        string(history.ToStatus),
		FromAccessLevel: string(history.FromAccessLevel),
		ToAccessLevel:   string(history.ToAccessLevel),
		Reason:          string(history.Reason),
		CreatedAt:       time.Now().UTC(),
	}

	if err := dbFromContext(ctx, repo.db).Create(&record).Error; err != nil {
		return fmt.Errorf("create user-patient history record: %w", err)
	}

	return nil
}
