package services

import (
	"appointment_system/internal/apps/appointment/models"
	"context"
	"time"
)

type CallParticipantRepository interface {
	Create(ctx context.Context, participant *models.CallParticipant) (*models.CallParticipant, error)
	GetByID(ctx context.Context, id uint64) (*models.CallParticipant, error)
	ListByRoom(ctx context.Context, roomID uint64) ([]*models.CallParticipant, error)
	// Update saves the participant's admission, media, mute, connection state
	// and presence times.
	Update(ctx context.Context, participant *models.CallParticipant) (*models.CallParticipant, error)
	Touch(ctx context.Context, id uint64, seenAt time.Time) error
}
