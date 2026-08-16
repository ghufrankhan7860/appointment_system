package services

import (
	"appointment_system/internal/apps/appointment/models"
	"context"
)

type UserRepository interface {
	Create(ctx context.Context, user *models.User) (*models.User, error)
}
