package services

import (
	"appointment_system/internal/apps/appointment/models"
	"context"
)

type UserPatientHistoryRepository interface {
	Create(ctx context.Context, history *models.UserPatientHistory) error
}
