package services

import (
	"appointment_system/internal/apps/appointment/models"
	"context"
)

type ClinicPatientRepository interface {
	Add(ctx context.Context, clinicPatient *models.ClinicPatient) error
	Get(ctx context.Context, doctorID uint64, patientID uint64) (*models.ClinicPatient, error)
}
