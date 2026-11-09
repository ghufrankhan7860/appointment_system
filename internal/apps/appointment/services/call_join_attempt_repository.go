package services

import (
	"context"
	"time"
)

type CallJoinAttemptRepository interface {
	Create(ctx context.Context, clientIP string, at time.Time) error
	CountSince(ctx context.Context, clientIP string, since time.Time) (int64, error)
	DeleteBefore(ctx context.Context, clientIP string, before time.Time) error
}
