package services

import (
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/utils"
	"context"
	"fmt"
)

type AuthService struct {
	users UserRepository
}

func NewAuthService(users UserRepository) (*AuthService, error) {
	if users == nil {
		return nil, constant.ErrNilUserRepository
	}
	return &AuthService{
		users: users,
	}, nil
}

func (as *AuthService) Register(ctx context.Context, user *models.User, password string) (*models.User, error) {

	passwordHash, err := utils.HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("error hashing password: %w", err)
	}
	user.PasswordHash = passwordHash
	createdUser, err := as.users.Create(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return createdUser, nil
}
