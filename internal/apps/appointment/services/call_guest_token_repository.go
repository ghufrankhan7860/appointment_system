package services

import (
	"appointment_system/internal/apps/appointment/models"
	"context"
	"time"
)

type CallGuestTokenRepository interface {
	Create(ctx context.Context, guestToken *models.CallGuestToken) (*models.CallGuestToken, error)
	// GetByTokenHashForUpdate locks the link until the transaction ends, so its
	// uses are counted one at a time.
	GetByTokenHashForUpdate(ctx context.Context, tokenHash string) (*models.CallGuestToken, error)
	IncrementUse(ctx context.Context, id uint64) error
	// Revoke changes only a link of the appointment that is not revoked yet.
	Revoke(ctx context.Context, appointmentID uint64, id uint64, revokedAt time.Time) error
}
