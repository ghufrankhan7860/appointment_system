package services

import (
	"appointment_system/internal/apps/appointment/models"
	"context"
	"time"
)

type StaffInviteRepository interface {
	Create(ctx context.Context, invite *models.StaffInvite) (*models.StaffInvite, error)
	GetByCodeHashForUpdate(ctx context.Context, codeHash string) (*models.StaffInvite, error)
	ListPendingByDoctor(ctx context.Context, doctorID uint64, now time.Time) ([]*models.StaffInvite, error)
	CountPendingByDoctor(ctx context.Context, doctorID uint64, now time.Time) (int64, error)
	MarkRedeemed(ctx context.Context, id uint64, redeemedByUserID uint64, redeemedAt time.Time) error
	Revoke(ctx context.Context, doctorID uint64, id uint64) error
	MarkExpired(ctx context.Context, id uint64) error
}
