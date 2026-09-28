package services

import (
	"appointment_system/internal/apps/appointment/models"
	"context"
)

type PatientRepository interface {
	Create(ctx context.Context, patient *models.Patient) (*models.Patient, error)
	GetByID(ctx context.Context, id uint64) (*models.Patient, error)
	GetByIDs(ctx context.Context, ids []uint64) (map[uint64]*models.Patient, error)
	Update(ctx context.Context, patient *models.Patient) (*models.Patient, error)
}
