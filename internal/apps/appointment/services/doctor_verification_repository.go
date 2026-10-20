package services

import (
	"appointment_system/internal/apps/appointment/models"
	"context"
)

type DoctorVerificationRepository interface {
	Create(ctx context.Context, verification *models.DoctorVerification) error
	ListByDoctor(ctx context.Context, doctorID uint64) ([]*models.DoctorVerification, error)
}
