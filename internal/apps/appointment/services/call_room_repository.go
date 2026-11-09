package services

import (
	"appointment_system/internal/apps/appointment/models"
	"context"
	"time"
)

type CallRoomRepository interface {
	// Create returns ErrCallRoomExists when the appointment already has a live
	// room and ErrCallJoinCodeExists when a live room has the same code.
	Create(ctx context.Context, room *models.CallRoom) (*models.CallRoom, error)
	GetByID(ctx context.Context, id uint64) (*models.CallRoom, error)
	GetLiveByAppointment(ctx context.Context, appointmentID uint64) (*models.CallRoom, error)
	// The ForUpdate reads lock the room until the transaction ends, so joins
	// and admissions into one room are checked one at a time.
	GetByIDForUpdate(ctx context.Context, id uint64) (*models.CallRoom, error)
	GetLiveByAppointmentForUpdate(ctx context.Context, appointmentID uint64) (*models.CallRoom, error)
	GetLiveByJoinCodeHashForUpdate(ctx context.Context, joinCodeHash string) (*models.CallRoom, error)
	ListByHost(ctx context.Context, hostUserID uint64, kind models.CallRoomKind, limit int) ([]*models.CallRoom, error)
	ListLive(ctx context.Context, limit int) ([]*models.CallRoom, error)
	CountLiveByHost(ctx context.Context, hostUserID uint64, kind models.CallRoomKind) (int64, error)
	CountCreatedByHostSince(ctx context.Context, hostUserID uint64, kind models.CallRoomKind, since time.Time) (int64, error)
	// MarkActive moves an open room to active; it leaves any other room as it is.
	MarkActive(ctx context.Context, id uint64, startedAt time.Time) (bool, error)
	// End changes only a live room.
	End(ctx context.Context, id uint64, reason models.CallEndReason, endedAt time.Time) error
}
