package services

import (
	"context"
	"time"
)

// StaffInviteAttemptRepository counts wrong staff invite codes per client IP;
// staff sign up before they have an account.
type StaffInviteAttemptRepository interface {
	Create(ctx context.Context, clientIP string, at time.Time) error
	CountSince(ctx context.Context, clientIP string, since time.Time) (int64, error)
	DeleteBefore(ctx context.Context, clientIP string, before time.Time) error
}
