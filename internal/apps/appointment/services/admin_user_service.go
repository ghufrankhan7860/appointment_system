package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/utils"
)

// AdminUserService creates admin accounts. Admins never come from public
// registration: the first one comes from the bootstrap script (CreateFirst),
// the rest from an existing admin (Create).
type AdminUserService struct {
	users  UserRepository
	logger Logger
}

func NewAdminUserService(
	users UserRepository,
	logger Logger,
) (*AdminUserService, error) {
	if users == nil {
		return nil, apperrors.ErrNilUserRepository
	}
	if logger == nil {
		return nil, apperrors.ErrNilLogger
	}

	return &AdminUserService{users: users, logger: logger}, nil
}

func (aus *AdminUserService) Create(
	ctx context.Context,
	user *models.User,
	password string,
) (*models.User, error) {
	if user == nil {
		return nil, apperrors.ErrNilAdminRequest
	}

	user.Name = strings.TrimSpace(user.Name)
	user.Email = strings.ToLower(strings.TrimSpace(user.Email))
	user.MobileNo = strings.TrimSpace(user.MobileNo)
	if user.Name == "" || user.Email == "" || user.MobileNo == "" {
		return nil, apperrors.Invalid(apperrors.ErrInvalidAdminInput, "", "name, email and mobileNo are required")
	}
	if len(password) < constant.MinAdminPasswordLength {
		return nil, apperrors.Invalid(apperrors.ErrInvalidAdminInput, "password", fmt.Sprintf("must be at least %d characters", constant.MinAdminPasswordLength))
	}

	passwordHash, err := utils.HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hash admin password: %w", err)
	}

	user.ID = 0
	user.Role = models.RoleAdmin
	user.PasswordHash = passwordHash

	createdUser, err := aus.users.Create(ctx, user)
	if err != nil {
		if errors.Is(err, apperrors.ErrUserAlreadyExists) {
			return nil, duplicateUserReason(err)
		}
		return nil, fmt.Errorf("create admin user: %w", err)
	}
	aus.logger.InfoContext(ctx, "admin user created", "user_id", createdUser.ID)

	return createdUser, nil
}

// CreateFirst creates the first admin and refuses once any admin exists, so the
// bootstrap script cannot be reused to add admins later.
func (aus *AdminUserService) CreateFirst(
	ctx context.Context,
	user *models.User,
	password string,
) (*models.User, error) {
	exists, err := aus.users.ExistsByRole(ctx, models.RoleAdmin)
	if err != nil {
		return nil, fmt.Errorf("check existing admin: %w", err)
	}
	if exists {
		return nil, apperrors.ErrAdminAlreadyExists
	}

	return aus.Create(ctx, user, password)
}
