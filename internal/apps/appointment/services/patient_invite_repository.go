package services

import (
	"appointment_system/internal/apps/appointment/models"
	"context"
	"time"
)

type PatientInviteRepository interface {
	Create(ctx context.Context, invite *models.PatientInvite) (*models.PatientInvite, error)
	// GetByCodeHashForUpdate locks the invite until the transaction ends, so
	// one code is redeemed once.
	GetByCodeHashForUpdate(ctx context.Context, codeHash string) (*models.PatientInvite, error)
	ListPendingByPatient(ctx context.Context, patientID uint64, now time.Time) ([]*models.PatientInvite, error)
	CountPendingByPatient(ctx context.Context, patientID uint64, now time.Time) (int64, error)
	MarkRedeemed(ctx context.Context, id uint64, redeemedByUserID uint64, redeemedAt time.Time) error
	// Revoke and MarkExpired change only a pending invite.
	Revoke(ctx context.Context, patientID uint64, id uint64) error
	MarkExpired(ctx context.Context, id uint64) error
}
