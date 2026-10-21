package services

import (
	"appointment_system/internal/apps/appointment/models"
	"context"
)

type OutboxRepository interface {
	Create(ctx context.Context, event *models.OutboxEvent) error
	ListUnpublished(ctx context.Context, limit int) ([]*models.OutboxEvent, error)
	MarkPublished(ctx context.Context, ids []uint64) error
}
