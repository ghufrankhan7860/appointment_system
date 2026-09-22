package controller

import (
	"appointment_system/internal/apps/appointment/apperrors"
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
		return nil, apperrors.ErrNilAuthService
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

func (ac *AuthController) Login(ctx context.Context, request *auth.LoginRequest) (*auth.LoginResponse, error) {
	user, accessToken, refreshToken, err := ac.authService.Login(ctx, request.Email, request.MobileNo, request.Password)
	if err != nil {
		return nil, err
	}
	return &auth.LoginResponse{
		ID:           user.ID,
		Name:         user.Name,
		Email:        user.Email,
		MobileNo:     user.MobileNo,
		Role:         string(user.Role),
		JwtToken:     accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func (ac *AuthController) Refresh(ctx context.Context, request *auth.RefreshRequest) (*auth.RefreshResponse, error) {
	accessToken, refreshToken, err := ac.authService.Refresh(ctx, request.Token)
	if err != nil {
		return nil, err
	}
	return &auth.RefreshResponse{
		JwtToken:     accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func (ac *AuthController) Logout(ctx context.Context, request *auth.RefreshRequest) error {
	return ac.authService.Logout(ctx, request.Token)
}
