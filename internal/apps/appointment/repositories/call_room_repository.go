package repositories

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CallRoomRepository struct {
	db *gorm.DB
}

func NewCallRoomRepository(db *gorm.DB) (*CallRoomRepository, error) {
	if db == nil {
		return nil, apperrors.ErrNilDB
	}
	return &CallRoomRepository{db: db}, nil
}

type callRoomRecord struct {
	ID               uint64     `gorm:"primaryKey;autoIncrement;column:id"`
	Kind             string     `gorm:"column:kind"`
	AppointmentID    *uint64    `gorm:"column:appointment_id"`
	HostUserID       uint64     `gorm:"column:host_user_id"`
	Title            string     `gorm:"column:title"`
	JoinCodeHash     *string    `gorm:"column:join_code_hash"`
	MaxParticipants  int        `gorm:"column:max_participants"`
	RequiresApproval bool       `gorm:"column:requires_approval"`
	ScheduledAt      *time.Time `gorm:"column:scheduled_at"`
	Status           string     `gorm:"column:status"`
	OpenedAt         time.Time  `gorm:"column:opened_at"`
	StartedAt        *time.Time `gorm:"column:started_at"`
	EndedAt          *time.Time `gorm:"column:ended_at"`
	EndReason        string     `gorm:"column:end_reason"`
	ExpiresAt        *time.Time `gorm:"column:expires_at"`
	CreatedAt        time.Time  `gorm:"column:created_at"`
	UpdatedAt        time.Time  `gorm:"column:updated_at"`
}

func (callRoomRecord) TableName() string {
	return "call_rooms"
}

var liveCallRoomStatuses = []string{string(models.CallRoomStatusOpen), string(models.CallRoomStatusActive)}

func (repo *CallRoomRepository) Create(ctx context.Context, room *models.CallRoom) (*models.CallRoom, error) {
	if room == nil {
		return nil, apperrors.ErrNilCallRequest
	}

	now := time.Now().UTC()
	record := callRoomRecord{
		Kind:             string(room.Kind),
		AppointmentID:    room.AppointmentID,
		HostUserID:       room.HostUserID,
		Title:            room.Title,
		MaxParticipants:  room.MaxParticipants,
		RequiresApproval: room.RequiresApproval,
		ScheduledAt:      room.ScheduledAt,
		Status:           string(room.Status),
		OpenedAt:         room.OpenedAt.UTC(),
		ExpiresAt:        room.ExpiresAt,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if room.JoinCodeHash != "" {
		record.JoinCodeHash = &room.JoinCodeHash
	}

	if err := dbFromContext(ctx, repo.db).Create(&record).Error; err != nil {
		if callRoomAppointmentLiveUnique.ViolatedBy(err) {
			return nil, apperrors.ErrCallRoomExists
		}
		if callRoomJoinCodeLiveUnique.ViolatedBy(err) {
			return nil, apperrors.ErrCallJoinCodeExists
		}
		return nil, fmt.Errorf("create call room record: %w", err)
	}

	return toCallRoomModel(record), nil
}

func (repo *CallRoomRepository) GetByID(ctx context.Context, id uint64) (*models.CallRoom, error) {
	return repo.first(ctx, false, "id = ?", id)
}

func (repo *CallRoomRepository) GetByIDForUpdate(ctx context.Context, id uint64) (*models.CallRoom, error) {
	return repo.first(ctx, true, "id = ?", id)
}

func (repo *CallRoomRepository) GetLiveByAppointment(ctx context.Context, appointmentID uint64) (*models.CallRoom, error) {
	return repo.first(ctx, false, "appointment_id = ? AND status IN ?", appointmentID, liveCallRoomStatuses)
}

func (repo *CallRoomRepository) GetLiveByAppointmentForUpdate(ctx context.Context, appointmentID uint64) (*models.CallRoom, error) {
	return repo.first(ctx, true, "appointment_id = ? AND status IN ?", appointmentID, liveCallRoomStatuses)
}

func (repo *CallRoomRepository) GetLiveByJoinCodeHashForUpdate(ctx context.Context, joinCodeHash string) (*models.CallRoom, error) {
	return repo.first(ctx, true, "join_code_hash = ? AND status IN ?", joinCodeHash, liveCallRoomStatuses)
}

func (repo *CallRoomRepository) ListByHost(ctx context.Context, hostUserID uint64, kind models.CallRoomKind, limit int) ([]*models.CallRoom, error) {
	var records []callRoomRecord

	if err := dbFromContext(ctx, repo.db).
		Where("host_user_id = ? AND kind = ?", hostUserID, string(kind)).
		Order("created_at DESC, id DESC").
		Limit(limit).
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list call rooms by host: %w", err)
	}

	return toCallRoomModels(records), nil
}

func (repo *CallRoomRepository) ListLive(ctx context.Context, limit int) ([]*models.CallRoom, error) {
	var records []callRoomRecord

	if err := dbFromContext(ctx, repo.db).
		Where("status IN ?", liveCallRoomStatuses).
		Order("opened_at, id").
		Limit(limit).
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list live call rooms: %w", err)
	}

	return toCallRoomModels(records), nil
}

func (repo *CallRoomRepository) CountLiveByHost(ctx context.Context, hostUserID uint64, kind models.CallRoomKind) (int64, error) {
	var count int64

	if err := dbFromContext(ctx, repo.db).
		Model(&callRoomRecord{}).
		Where("host_user_id = ? AND kind = ? AND status IN ?", hostUserID, string(kind), liveCallRoomStatuses).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count live call rooms by host: %w", err)
	}

	return count, nil
}

func (repo *CallRoomRepository) CountCreatedByHostSince(ctx context.Context, hostUserID uint64, kind models.CallRoomKind, since time.Time) (int64, error) {
	var count int64

	if err := dbFromContext(ctx, repo.db).
		Model(&callRoomRecord{}).
		Where("host_user_id = ? AND kind = ? AND created_at > ?", hostUserID, string(kind), since.UTC()).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count call rooms created by host: %w", err)
	}

	return count, nil
}

func (repo *CallRoomRepository) MarkActive(ctx context.Context, id uint64, startedAt time.Time) (bool, error) {
	result := dbFromContext(ctx, repo.db).
		Model(&callRoomRecord{}).
		Where("id = ? AND status = ?", id, string(models.CallRoomStatusOpen)).
		Updates(map[string]interface{}{
			"status":     string(models.CallRoomStatusActive),
			"started_at": startedAt.UTC(),
			"updated_at": time.Now().UTC(),
		})
	if result.Error != nil {
		return false, fmt.Errorf("mark call room active: %w", result.Error)
	}

	return result.RowsAffected > 0, nil
}

func (repo *CallRoomRepository) End(ctx context.Context, id uint64, reason models.CallEndReason, endedAt time.Time) error {
	result := dbFromContext(ctx, repo.db).
		Model(&callRoomRecord{}).
		Where("id = ? AND status IN ?", id, liveCallRoomStatuses).
		Updates(map[string]interface{}{
			"status":     string(models.CallRoomStatusEnded),
			"end_reason": string(reason),
			"ended_at":   endedAt.UTC(),
			"updated_at": time.Now().UTC(),
		})
	if result.Error != nil {
		return fmt.Errorf("end call room: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrCallRoomNotFound
	}

	return nil
}

func (repo *CallRoomRepository) first(ctx context.Context, forUpdate bool, condition string, args ...any) (*models.CallRoom, error) {
	var record callRoomRecord

	query := dbFromContext(ctx, repo.db)
	if forUpdate {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.Where(condition, args...).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrCallRoomNotFound
		}
		return nil, fmt.Errorf("get call room: %w", err)
	}

	return toCallRoomModel(record), nil
}

func toCallRoomModels(records []callRoomRecord) []*models.CallRoom {
	result := make([]*models.CallRoom, 0, len(records))
	for _, record := range records {
		result = append(result, toCallRoomModel(record))
	}
	return result
}

func toCallRoomModel(record callRoomRecord) *models.CallRoom {
	room := &models.CallRoom{
		ID:               record.ID,
		Kind:             models.CallRoomKind(record.Kind),
		AppointmentID:    record.AppointmentID,
		HostUserID:       record.HostUserID,
		Title:            record.Title,
		MaxParticipants:  record.MaxParticipants,
		RequiresApproval: record.RequiresApproval,
		ScheduledAt:      record.ScheduledAt,
		Status:           models.CallRoomStatus(record.Status),
		OpenedAt:         record.OpenedAt,
		StartedAt:        record.StartedAt,
		EndedAt:          record.EndedAt,
		EndReason:        models.CallEndReason(record.EndReason),
		ExpiresAt:        record.ExpiresAt,
		CreatedAt:        record.CreatedAt,
		UpdatedAt:        record.UpdatedAt,
	}
	if record.JoinCodeHash != nil {
		room.JoinCodeHash = *record.JoinCodeHash
	}

	return room
}
