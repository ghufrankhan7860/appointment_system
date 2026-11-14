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

type ClinicPatientRepository struct {
	db *gorm.DB
}

func NewClinicPatientRepository(db *gorm.DB) (*ClinicPatientRepository, error) {
	if db == nil {
		return nil, apperrors.ErrNilDB
	}
	return &ClinicPatientRepository{db: db}, nil
}

type clinicPatientRecord struct {
	DoctorID        uint64    `gorm:"primaryKey;column:doctor_id"`
	PatientID       uint64    `gorm:"primaryKey;column:patient_id"`
	Source          string    `gorm:"column:source"`
	CreatedByUserID uint64    `gorm:"column:created_by_user_id"`
	CreatedAt       time.Time `gorm:"column:created_at"`
}

func (clinicPatientRecord) TableName() string {
	return "clinic_patients"
}

// Add records that the doctor's desk may book for the patient. A patient the
// desk already has keeps their first source.
func (repo *ClinicPatientRepository) Add(ctx context.Context, clinicPatient *models.ClinicPatient) error {
	if clinicPatient == nil {
		return apperrors.ErrInvalidDeskInput
	}

	record := clinicPatientRecord{
		DoctorID:        clinicPatient.DoctorID,
		PatientID:       clinicPatient.PatientID,
		Source:          string(clinicPatient.Source),
		CreatedByUserID: clinicPatient.CreatedByUserID,
		CreatedAt:       time.Now().UTC(),
	}

	if err := dbFromContext(ctx, repo.db).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&record).Error; err != nil {
		return fmt.Errorf("add clinic patient: %w", err)
	}

	return nil
}

// Get returns the desk's record of the patient, or ErrPatientNotAtClinic.
func (repo *ClinicPatientRepository) Get(ctx context.Context, doctorID uint64, patientID uint64) (*models.ClinicPatient, error) {
	var record clinicPatientRecord

	if err := dbFromContext(ctx, repo.db).
		Where("doctor_id = ? AND patient_id = ?", doctorID, patientID).
		First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrPatientNotAtClinic
		}
		return nil, fmt.Errorf("get clinic patient: %w", err)
	}

	return &models.ClinicPatient{
		DoctorID:        record.DoctorID,
		PatientID:       record.PatientID,
		Source:          models.ClinicPatientSource(record.Source),
		CreatedByUserID: record.CreatedByUserID,
		CreatedAt:       record.CreatedAt,
	}, nil
}
