package services

import (
	"appointment_system/internal/apps/appointment/models"
	"context"
)

type UserPatientRepository interface {
	Create(ctx context.Context, userPatient *models.UserPatient) (*models.UserPatient, error)
	GetByUserAndPatient(ctx context.Context, userID uint64, patientID uint64) (*models.UserPatient, error)
	GetActiveByUserAndPatient(ctx context.Context, userID uint64, patientID uint64) (*models.UserPatient, error)
	ListByPatient(ctx context.Context, patientID uint64, limit int, offset int) ([]*models.UserPatient, error)
	ListByUser(ctx context.Context, userID uint64, limit int, offset int) ([]*models.UserPatient, error)
	ListActiveByUser(ctx context.Context, userID uint64, limit int, offset int) ([]*models.UserPatient, error)
	ListActiveByPatient(ctx context.Context, patientID uint64) ([]*models.UserPatient, error)
	// ListActiveByPatientForUpdate locks the patient's active links until the
	// transaction ends, so concurrent membership changes see each other.
	ListActiveByPatientForUpdate(ctx context.Context, patientID uint64) ([]*models.UserPatient, error)
	Update(ctx context.Context, userPatient *models.UserPatient) (*models.UserPatient, error)
	Revoke(ctx context.Context, userID uint64, patientID uint64) (*models.UserPatient, error)
}
