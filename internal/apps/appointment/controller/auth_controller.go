package controller

import (
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/dtos/auth"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
	"context"
)

type AuthController struct {
	authService *services.AuthService
}

func NewAuthController(authService *services.AuthService) (*AuthController, error) {
	if authService == nil {
		return nil, constant.ErrNilAuthService
	}
	return &AuthController{
		authService: authService,
	}, nil
}

func (ac *AuthController) Register(ctx context.Context, request *auth.RegisterRequest) (*auth.RegisterResponse, error) {
	user := models.User{
		Name:     request.Name,
		Email:    request.Email,
		MobileNo: request.MobileNo,
		Role:     models.Role(request.Role),
	}
	createdUser, err := ac.authService.Register(ctx, &user, request.Password)
	if err != nil {
		return nil, err
	}
	return &auth.RegisterResponse{
		ID:       createdUser.ID,
		Name:     createdUser.Name,
		Email:    createdUser.Email,
		MobileNo: createdUser.MobileNo,
		Role:     string(createdUser.Role),
	}, nil
}
