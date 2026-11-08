package services

import (
	"appointment_system/internal/apps/appointment/models"
	"context"
	"time"
)

type SessionRepository interface {
	Create(ctx context.Context, session *models.Session) error
	GetByTokenHash(ctx context.Context, tokenHash string) (*models.Session, error)
	RevokeByID(ctx context.Context, ID uint64) error
	Rotate(ctx context.Context, id uint64, oldTokenHash string, session *models.Session) error
	ListActiveByUser(ctx context.Context, userID uint64, now time.Time) ([]*models.Session, error)
	ListByUser(ctx context.Context, userID uint64, limit int, offset int) ([]*models.Session, error)
	RevokeByUserAndID(ctx context.Context, userID uint64, id uint64) error
	RevokeActiveByUser(ctx context.Context, userID uint64, exceptID uint64) (int64, error)
	DeleteEndedBefore(ctx context.Context, before time.Time, limit int) (int64, error)
}
