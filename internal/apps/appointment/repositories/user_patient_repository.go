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

type UserPatientRepository struct {
	db *gorm.DB
}

func NewUserPatientRepository(db *gorm.DB) (*UserPatientRepository, error) {
	if db == nil {
		return nil, apperrors.ErrNilDB
	}
	return &UserPatientRepository{db: db}, nil
}

type userPatientRecord struct {
	UserID              uint64    `gorm:"primaryKey;column:user_id"`
	PatientID           uint64    `gorm:"primaryKey;column:patient_id"`
	PatientRelationship string    `gorm:"column:patient_relationship"`
	AccessLevel         string    `gorm:"column:access_level"`
	Status              string    `gorm:"column:status"`
	CreatedAt           time.Time `gorm:"column:created_at"`
	UpdatedAt           time.Time `gorm:"column:updated_at"`
}

func (userPatientRecord) TableName() string {
	return "user_patient"
}

func (repo *UserPatientRepository) Create(ctx context.Context, userPatient *models.UserPatient) (*models.UserPatient, error) {
	if userPatient == nil {
		return nil, apperrors.ErrNilUserPatientRequest
	}

	now := time.Now().UTC()
	record := userPatientRecord{
		UserID:              userPatient.UserID,
		PatientID:           userPatient.PatientID,
		PatientRelationship: string(userPatient.PatientRelationship),
		AccessLevel:         string(userPatient.AccessLevel),
		Status:              string(userPatient.Status),
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	if err := repo.db.WithContext(ctx).Create(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, apperrors.ErrUserPatientAlreadyExists
		}
		return nil, fmt.Errorf("create user-patient record: %w", err)
	}

	return toUserPatientModel(record), nil
}

func (repo *UserPatientRepository) GetByUserAndPatient(ctx context.Context, userID uint64, patientID uint64) (*models.UserPatient, error) {
	var record userPatientRecord

	if err := repo.db.WithContext(ctx).
		Where("user_id = ? AND patient_id = ?", userID, patientID).
		First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrUserPatientNotFound
		}
		return nil, fmt.Errorf("get user-patient by ids: %w", err)
	}

	return toUserPatientModel(record), nil
}

func (repo *UserPatientRepository) ListByPatient(ctx context.Context, patientID uint64, limit int, offset int) ([]*models.UserPatient, error) {
	var records []userPatientRecord

	if err := repo.db.WithContext(ctx).
		Where("patient_id = ?", patientID).
		Order("user_id ASC").
		Limit(limit).
		Offset(offset).
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list user-patients by patient: %w", err)
	}

	result := make([]*models.UserPatient, 0, len(records))
	for _, record := range records {
		result = append(result, toUserPatientModel(record))
	}
	return result, nil
}

func (repo *UserPatientRepository) ListByUser(ctx context.Context, userID uint64, limit int, offset int) ([]*models.UserPatient, error) {
	var records []userPatientRecord

	if err := repo.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("patient_id ASC").
		Limit(limit).
		Offset(offset).
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list user-patients by user: %w", err)
	}

	result := make([]*models.UserPatient, 0, len(records))
	for _, record := range records {
		result = append(result, toUserPatientModel(record))
	}
	return result, nil
}

func (repo *UserPatientRepository) Update(ctx context.Context, userPatient *models.UserPatient) (*models.UserPatient, error) {
	if userPatient == nil {
		return nil, apperrors.ErrNilUserPatientRequest
	}

	now := time.Now().UTC()
	updates := map[string]interface{}{
		"patient_relationship": string(userPatient.PatientRelationship),
		"access_level":         string(userPatient.AccessLevel),
		"status":               string(userPatient.Status),
		"updated_at":           now,
	}

	result := repo.db.WithContext(ctx).
		Model(&userPatientRecord{}).
		Where("user_id = ? AND patient_id = ?", userPatient.UserID, userPatient.PatientID).
		Updates(updates)
	if result.Error != nil {
		return nil, fmt.Errorf("update user-patient record: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, apperrors.ErrUserPatientNotFound
	}

	return repo.GetByUserAndPatient(ctx, userPatient.UserID, userPatient.PatientID)
}

func toUserPatientModel(record userPatientRecord) *models.UserPatient {
	return &models.UserPatient{
		UserID:              record.UserID,
		PatientID:           record.PatientID,
		PatientRelationship: models.Relationship(record.PatientRelationship),
		AccessLevel:         models.AccessLevel(record.AccessLevel),
		Status:              models.UserPatientStatus(record.Status),
		CreatedAt:           record.CreatedAt,
		UpdatedAt:           record.UpdatedAt,
	}
}
