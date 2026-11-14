package services

import (
	"appointment_system/internal/apps/appointment/models"
	"context"
	"time"
)

type StaffRepository interface {
	Create(ctx context.Context, staff *models.DoctorStaff) (*models.DoctorStaff, error)
	GetActiveByUser(ctx context.Context, userID uint64) (*models.DoctorStaff, error)
	ListActiveByDoctor(ctx context.Context, doctorID uint64) ([]*models.StaffMember, error)
	Remove(ctx context.Context, doctorID uint64, userID uint64, removedByUserID uint64, removedAt time.Time) error
}
