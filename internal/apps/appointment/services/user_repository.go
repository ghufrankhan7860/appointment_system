package services

import (
	"context"

	"appointment_system/internal/apps/appointment/models"
)

type UserRepository interface {
	Create(ctx context.Context, user *models.User) (*models.User, error)
	FindByEmail(ctx context.Context, email string) (*models.User, error)
	FindByMobile(ctx context.Context, mobileNo string) (*models.User, error)
	GetByID(ctx context.Context, id uint64) (*models.User, error)
	ExistsByRole(ctx context.Context, role models.Role) (bool, error)
}
