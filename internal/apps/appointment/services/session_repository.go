package services

import (
	"appointment_system/internal/apps/appointment/models"
	"context"
)

type SessionRepository interface {
	Create(ctx context.Context, session *models.Session) error
	GetByTokenHash(ctx context.Context, tokenHash string) (*models.Session, error)
	RevokeByID(ctx context.Context, ID uint64) error
}
