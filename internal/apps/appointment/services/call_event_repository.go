package services

import (
	"appointment_system/internal/apps/appointment/models"
	"context"
)

type CallEventRepository interface {
	Create(ctx context.Context, event *models.CallEvent) error
}
