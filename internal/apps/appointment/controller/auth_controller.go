package controller

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/dtos/auth"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
	"context"
)

type AuthController struct {
	authService    *services.AuthService
	sessionService *services.SessionService
}

func NewAuthController(
	authService *services.AuthService,
	sessionService *services.SessionService,
) (*AuthController, error) {
	if authService == nil {
		return nil, apperrors.ErrNilAuthService
	}
	if sessionService == nil {
		return nil, apperrors.ErrNilSessionService
	}
	return &AuthController{
		authService:    authService,
		sessionService: sessionService,
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

func (ac *AuthController) Login(
	ctx context.Context,
	request *auth.LoginRequest,
	clientHeaders *auth.ClientHeaders,
	clientIP string,
) (*auth.LoginResponse, error) {
	user, accessToken, refreshToken, err := ac.authService.Login(ctx, request.Email, request.MobileNo, request.Password, toSessionClient(clientHeaders, clientIP))
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

func (ac *AuthController) Refresh(
	ctx context.Context,
	request *auth.RefreshRequest,
	clientHeaders *auth.ClientHeaders,
	clientIP string,
) (*auth.RefreshResponse, error) {
	accessToken, refreshToken, err := ac.authService.Refresh(ctx, request.Token, toSessionClient(clientHeaders, clientIP))
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

func (ac *AuthController) ListSessions(ctx context.Context) (*auth.ListSessionsResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}

	sessions, err := ac.sessionService.List(ctx, userID)
	if err != nil {
		return nil, err
	}

	currentSessionID := actingSessionID(ctx)
	response := auth.ListSessionsResponse{
		Sessions: make([]auth.SessionResponse, 0, len(sessions)),
	}
	for _, session := range sessions {
		response.Sessions = append(response.Sessions, toSessionResponse(session, currentSessionID))
	}

	return &response, nil
}

func (ac *AuthController) RevokeSession(ctx context.Context, request *auth.RevokeSessionRequest) error {
	userID, err := actingUserID(ctx)
	if err != nil {
		return err
	}

	return ac.sessionService.Revoke(ctx, userID, request.ID)
}

// RevokeOtherSessions needs the current session's ID, so a token issued before
// session IDs were carried is refused as ErrPrincipalNotFound until it is refreshed.
func (ac *AuthController) RevokeOtherSessions(ctx context.Context) (*auth.RevokeOtherSessionsResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	currentSessionID := actingSessionID(ctx)
	if currentSessionID == 0 {
		return nil, apperrors.ErrPrincipalNotFound
	}

	revoked, err := ac.sessionService.RevokeOthers(ctx, userID, currentSessionID)
	if err != nil {
		return nil, err
	}

	return &auth.RevokeOtherSessionsResponse{Revoked: revoked}, nil
}
