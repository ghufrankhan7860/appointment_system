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

type PatientRepository struct {
	db *gorm.DB
}

type patientRecord struct {
	ID               uint64    `gorm:"primaryKey;autoIncrement;column:id"`
	Name             string    `gorm:"column:name"`
	Gender           string    `gorm:"column:gender"`
	DateOfBirth      time.Time `gorm:"column:date_of_birth"`
	BloodType        string    `gorm:"column:blood_type"`
	EmergencyContact string    `gorm:"column:emergency_contact"`
	CreatedAt        time.Time `gorm:"column:created_at"`
	UpdatedAt        time.Time `gorm:"column:updated_at"`
}

func (patientRecord) TableName() string {
	return "patients"
}

func NewPatientRepository(db *gorm.DB) (*PatientRepository, error) {
	if db == nil {
		return nil, apperrors.ErrNilDB
	}

	return &PatientRepository{db: db}, nil
}

func (pr *PatientRepository) Create(
	ctx context.Context,
	patient *models.Patient,
) (*models.Patient, error) {
	if patient == nil {
		return nil, apperrors.ErrNilPatientRequest
	}

	now := time.Now().UTC()

	record := &patientRecord{
		Name:             patient.Name,
		Gender:           string(patient.Gender),
		DateOfBirth:      patient.DateOfBirth,
		BloodType:        string(patient.BloodType),
		EmergencyContact: patient.EmergencyContact,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	if err := dbFromContext(ctx, pr.db).Create(record).Error; err != nil {
		return nil, fmt.Errorf("create patient record: %w", err)
	}

	return &models.Patient{
		ID:               record.ID,
		Name:             record.Name,
		Gender:           models.Gender(record.Gender),
		DateOfBirth:      record.DateOfBirth,
		BloodType:        models.BloodType(record.BloodType),
		EmergencyContact: record.EmergencyContact,
		CreatedAt:        record.CreatedAt,
		UpdatedAt:        record.UpdatedAt,
	}, nil
}

func (pr *PatientRepository) GetByID(
	ctx context.Context,
	id uint64,
) (*models.Patient, error) {
	var record patientRecord

	if err := dbFromContext(ctx, pr.db).
		Where("id = ?", id).
		First(&record).
		Error; err != nil {

		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrPatientNotFound
		}

		return nil, fmt.Errorf("get patient by id: %w", err)
	}

	patient := &models.Patient{
		ID:               record.ID,
		Name:             record.Name,
		Gender:           models.Gender(record.Gender),
		DateOfBirth:      record.DateOfBirth,
		BloodType:        models.BloodType(record.BloodType),
		EmergencyContact: record.EmergencyContact,
		CreatedAt:        record.CreatedAt,
		UpdatedAt:        record.UpdatedAt,
	}

	return patient, nil
}

func (pr *PatientRepository) GetByIDs(
	ctx context.Context,
	ids []uint64,
) (map[uint64]*models.Patient, error) {
	result := make(map[uint64]*models.Patient, len(ids))
	if len(ids) == 0 {
		return result, nil
	}

	var records []patientRecord
	if err := dbFromContext(ctx, pr.db).
		Where("id IN ?", ids).
		Find(&records).
		Error; err != nil {
		return nil, fmt.Errorf("get patients by ids: %w", err)
	}

	for _, record := range records {
		record := record
		result[record.ID] = &models.Patient{
			ID:               record.ID,
			Name:             record.Name,
			Gender:           models.Gender(record.Gender),
			DateOfBirth:      record.DateOfBirth,
			BloodType:        models.BloodType(record.BloodType),
			EmergencyContact: record.EmergencyContact,
			CreatedAt:        record.CreatedAt,
			UpdatedAt:        record.UpdatedAt,
		}
	}

	return result, nil
}

func (pr *PatientRepository) Update(
	ctx context.Context,
	patient *models.Patient,
) (*models.Patient, error) {
	if patient == nil {
		return nil, apperrors.ErrNilPatientRequest
	}
	if patient.ID == 0 {
		return nil, fmt.Errorf("update patient without id: %w", apperrors.ErrInvalidPatientInput)
	}

	now := time.Now().UTC()
	updates := map[string]interface{}{
		"name":              patient.Name,
		"gender":            string(patient.Gender),
		"date_of_birth":     patient.DateOfBirth,
		"blood_type":        string(patient.BloodType),
		"emergency_contact": patient.EmergencyContact,
		"updated_at":        now,
	}

	result := dbFromContext(ctx, pr.db).
		Model(&patientRecord{}).
		Where("id = ?", patient.ID).
		Updates(updates)
	if result.Error != nil {
		return nil, fmt.Errorf("update patient record: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, apperrors.ErrPatientNotFound
	}

	return pr.GetByID(ctx, patient.ID)
}
