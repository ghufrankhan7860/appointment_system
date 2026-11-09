package repositories

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type CallParticipantRepository struct {
	db *gorm.DB
}

func NewCallParticipantRepository(db *gorm.DB) (*CallParticipantRepository, error) {
	if db == nil {
		return nil, apperrors.ErrNilDB
	}
	return &CallParticipantRepository{db: db}, nil
}

type callParticipantRecord struct {
	ID              uint64     `gorm:"primaryKey;autoIncrement;column:id"`
	CallRoomID      uint64     `gorm:"column:call_room_id"`
	UserID          *uint64    `gorm:"column:user_id"`
	GuestTokenID    *uint64    `gorm:"column:guest_token_id"`
	PeerID          string     `gorm:"column:peer_id"`
	DisplayName     string     `gorm:"column:display_name"`
	Role            string     `gorm:"column:role"`
	AdmissionStatus string     `gorm:"column:admission_status"`
	MediaMode       string     `gorm:"column:media_mode"`
	MutedByHost     bool       `gorm:"column:muted_by_host"`
	ConnectionState string     `gorm:"column:connection_state"`
	JoinedAt        *time.Time `gorm:"column:joined_at"`
	LastSeenAt      *time.Time `gorm:"column:last_seen_at"`
	LeftAt          *time.Time `gorm:"column:left_at"`
	CreatedAt       time.Time  `gorm:"column:created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at"`
}

func (callParticipantRecord) TableName() string {
	return "call_participants"
}

func (repo *CallParticipantRepository) Create(ctx context.Context, participant *models.CallParticipant) (*models.CallParticipant, error) {
	if participant == nil {
		return nil, apperrors.ErrNilCallRequest
	}

	now := time.Now().UTC()
	record := callParticipantRecord{
		CallRoomID:      participant.CallRoomID,
		UserID:          participant.UserID,
		GuestTokenID:    participant.GuestTokenID,
		PeerID:          participant.PeerID,
		DisplayName:     participant.DisplayName,
		Role:            string(participant.Role),
		AdmissionStatus: string(participant.AdmissionStatus),
		MediaMode:       string(participant.MediaMode),
		MutedByHost:     participant.MutedByHost,
		ConnectionState: string(participant.ConnectionState),
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := dbFromContext(ctx, repo.db).Create(&record).Error; err != nil {
		return nil, fmt.Errorf("create call participant record: %w", err)
	}

	return toCallParticipantModel(record), nil
}

func (repo *CallParticipantRepository) GetByID(ctx context.Context, id uint64) (*models.CallParticipant, error) {
	var record callParticipantRecord

	if err := dbFromContext(ctx, repo.db).Where("id = ?", id).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrCallParticipantNotFound
		}
		return nil, fmt.Errorf("get call participant by id: %w", err)
	}

	return toCallParticipantModel(record), nil
}

func (repo *CallParticipantRepository) ListByRoom(ctx context.Context, roomID uint64) ([]*models.CallParticipant, error) {
	var records []callParticipantRecord

	if err := dbFromContext(ctx, repo.db).
		Where("call_room_id = ?", roomID).
		Order("created_at, id").
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list call participants by room: %w", err)
	}

	result := make([]*models.CallParticipant, 0, len(records))
	for _, record := range records {
		result = append(result, toCallParticipantModel(record))
	}
	return result, nil
}

func (repo *CallParticipantRepository) Update(ctx context.Context, participant *models.CallParticipant) (*models.CallParticipant, error) {
	if participant == nil {
		return nil, apperrors.ErrNilCallRequest
	}

	result := dbFromContext(ctx, repo.db).
		Model(&callParticipantRecord{}).
		Where("id = ?", participant.ID).
		Updates(map[string]interface{}{
			"admission_status": string(participant.AdmissionStatus),
			"media_mode":       string(participant.MediaMode),
			"muted_by_host":    participant.MutedByHost,
			"connection_state": string(participant.ConnectionState),
			"joined_at":        participant.JoinedAt,
			"last_seen_at":     participant.LastSeenAt,
			"left_at":          participant.LeftAt,
			"updated_at":       time.Now().UTC(),
		})
	if result.Error != nil {
		return nil, fmt.Errorf("update call participant: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, apperrors.ErrCallParticipantNotFound
	}

	return repo.GetByID(ctx, participant.ID)
}

func (repo *CallParticipantRepository) Touch(ctx context.Context, id uint64, seenAt time.Time) error {
	if err := dbFromContext(ctx, repo.db).
		Model(&callParticipantRecord{}).
		Where("id = ?", id).
		Update("last_seen_at", seenAt.UTC()).Error; err != nil {
		return fmt.Errorf("touch call participant: %w", err)
	}

	return nil
}

func toCallParticipantModel(record callParticipantRecord) *models.CallParticipant {
	return &models.CallParticipant{
		ID:              record.ID,
		CallRoomID:      record.CallRoomID,
		UserID:          record.UserID,
		GuestTokenID:    record.GuestTokenID,
		PeerID:          record.PeerID,
		DisplayName:     record.DisplayName,
		Role:            models.CallParticipantRole(record.Role),
		AdmissionStatus: models.CallAdmissionStatus(record.AdmissionStatus),
		MediaMode:       models.CallMediaMode(record.MediaMode),
		MutedByHost:     record.MutedByHost,
		ConnectionState: models.CallConnectionState(record.ConnectionState),
		JoinedAt:        record.JoinedAt,
		LastSeenAt:      record.LastSeenAt,
		LeftAt:          record.LeftAt,
		CreatedAt:       record.CreatedAt,
		UpdatedAt:       record.UpdatedAt,
	}
}
