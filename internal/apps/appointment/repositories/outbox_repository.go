package repositories

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type OutboxRepository struct {
	db *gorm.DB
}

func NewOutboxRepository(db *gorm.DB) (*OutboxRepository, error) {
	if db == nil {
		return nil, apperrors.ErrNilDB
	}
	return &OutboxRepository{db: db}, nil
}

type outboxRecord struct {
	ID            uint64     `gorm:"primaryKey;autoIncrement;column:id"`
	AggregateType string     `gorm:"column:aggregate_type"`
	AggregateID   uint64     `gorm:"column:aggregate_id"`
	EventType     string     `gorm:"column:event_type"`
	Payload       string     `gorm:"column:payload"`
	CreatedAt     time.Time  `gorm:"column:created_at"`
	PublishedAt   *time.Time `gorm:"column:published_at"`
}

func (outboxRecord) TableName() string {
	return "outbox"
}

// Create writes an event to publish later. Callers run it inside the
// transaction of the change it describes.
func (repo *OutboxRepository) Create(ctx context.Context, event *models.OutboxEvent) error {
	if event == nil {
		return apperrors.ErrNilAppointmentRequest
	}

	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("encode outbox payload: %w", err)
	}

	record := outboxRecord{
		AggregateType: event.AggregateType,
		AggregateID:   event.AggregateID,
		EventType:     event.EventType,
		Payload:       string(payload),
		CreatedAt:     time.Now().UTC(),
	}

	if err := dbFromContext(ctx, repo.db).Create(&record).Error; err != nil {
		return fmt.Errorf("create outbox record: %w", err)
	}

	return nil
}

// ListUnpublished returns the oldest unpublished events and locks them until
// the surrounding transaction ends. Rows another consumer already holds are
// skipped, so two consumers never publish the same event at once.
func (repo *OutboxRepository) ListUnpublished(ctx context.Context, limit int) ([]*models.OutboxEvent, error) {
	var records []outboxRecord

	if err := dbFromContext(ctx, repo.db).
		Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		Where("published_at IS NULL").
		Order("id ASC").
		Limit(limit).
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list unpublished outbox records: %w", err)
	}

	result := make([]*models.OutboxEvent, 0, len(records))
	for _, record := range records {
		var payload map[string]any
		if err := json.Unmarshal([]byte(record.Payload), &payload); err != nil {
			return nil, fmt.Errorf("decode outbox payload %d: %w", record.ID, err)
		}
		result = append(result, &models.OutboxEvent{
			ID:            record.ID,
			AggregateType: record.AggregateType,
			AggregateID:   record.AggregateID,
			EventType:     record.EventType,
			Payload:       payload,
			CreatedAt:     record.CreatedAt,
			PublishedAt:   record.PublishedAt,
		})
	}

	return result, nil
}

func (repo *OutboxRepository) MarkPublished(ctx context.Context, ids []uint64) error {
	if len(ids) == 0 {
		return nil
	}

	if err := dbFromContext(ctx, repo.db).
		Model(&outboxRecord{}).
		Where("id IN ? AND published_at IS NULL", ids).
		Update("published_at", time.Now().UTC()).Error; err != nil {
		return fmt.Errorf("mark outbox records published: %w", err)
	}

	return nil
}
