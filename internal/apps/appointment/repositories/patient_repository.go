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

	if err := pr.db.WithContext(ctx).Create(record).Error; err != nil {
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

	if err := pr.db.WithContext(ctx).
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
