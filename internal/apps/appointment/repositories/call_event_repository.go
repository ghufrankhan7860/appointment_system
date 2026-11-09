package repositories

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type CallEventRepository struct {
	db *gorm.DB
}

func NewCallEventRepository(db *gorm.DB) (*CallEventRepository, error) {
	if db == nil {
		return nil, apperrors.ErrNilDB
	}
	return &CallEventRepository{db: db}, nil
}

type callEventRecord struct {
	ID                uint64    `gorm:"primaryKey;autoIncrement;column:id"`
	CallRoomID        uint64    `gorm:"column:call_room_id"`
	CallParticipantID *uint64   `gorm:"column:call_participant_id"`
	EventType         string    `gorm:"column:event_type"`
	Detail            string    `gorm:"column:detail"`
	CreatedAt         time.Time `gorm:"column:created_at"`
}

func (callEventRecord) TableName() string {
	return "call_events"
}

func (repo *CallEventRepository) Create(ctx context.Context, event *models.CallEvent) error {
	if event == nil {
		return apperrors.ErrNilCallRequest
	}

	record := callEventRecord{
		CallRoomID:        event.CallRoomID,
		CallParticipantID: event.CallParticipantID,
		EventType:         string(event.EventType),
		Detail:            event.Detail,
		CreatedAt:         time.Now().UTC(),
	}

	if err := dbFromContext(ctx, repo.db).Create(&record).Error; err != nil {
		return fmt.Errorf("create call event record: %w", err)
	}

	return nil
}
