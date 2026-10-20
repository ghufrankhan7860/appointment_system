package services

import (
	"appointment_system/internal/apps/appointment/models"
	"context"
	"time"
)

type DoctorRepository interface {
	Create(ctx context.Context, doctor *models.Doctor) (*models.Doctor, error)
	GetByID(ctx context.Context, id uint64) (*models.Doctor, error)
	GetByUserID(ctx context.Context, userID uint64) (*models.Doctor, error)
	Update(ctx context.Context, doctor *models.Doctor) (*models.Doctor, error)
	UpdateStatus(ctx context.Context, id uint64, fromStatus models.VerificationStatus, toStatus models.VerificationStatus, verifiedAt *time.Time) error
	ListByStatus(ctx context.Context, status models.VerificationStatus, limit int, offset int) ([]*models.Doctor, error)
	Search(ctx context.Context, search models.DoctorSearch) ([]*models.Doctor, error)
}
