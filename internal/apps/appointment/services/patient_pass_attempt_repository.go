package services

import (
	"context"
	"time"
)

type PatientPassAttemptRepository interface {
	Create(ctx context.Context, userID uint64, at time.Time) error
	CountSince(ctx context.Context, userID uint64, since time.Time) (int64, error)
	DeleteBefore(ctx context.Context, userID uint64, before time.Time) error
}
