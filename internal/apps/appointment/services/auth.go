package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/utils"
)

type AuthService struct {
	users    UserRepository
	sessions SessionRepository
	tokens   TokenGenerator
	tx       TransactionManager
}

type TokenGenerator interface {
	Generate(user models.User) (string, error)
}

func NewAuthService(
	users UserRepository,
	sessions SessionRepository,
	tokens TokenGenerator,
	tx TransactionManager,
) (*AuthService, error) {
	if users == nil {
		return nil, apperrors.ErrNilUserRepository
	}
	if sessions == nil {
		return nil, apperrors.ErrNilSessionRepository
	}
	if tokens == nil {
		return nil, apperrors.ErrNilTokenGenerator
	}
	if tx == nil {
		return nil, apperrors.ErrNilTransactionManager
	}

	return &AuthService{
		users:    users,
		sessions: sessions,
		tokens:   tokens,
		tx:       tx,
	}, nil
}

func (as *AuthService) Register(
	ctx context.Context,
	user *models.User,
	password string,
) (*models.User, error) {
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

func (as *AuthService) Login(
	ctx context.Context,
	email *string,
	mobileNo *string,
	password string,
) (*models.User, string, string, error) {
	identifier, err := loginIdentifier(email, mobileNo)
	if err != nil {
		return nil, "", "", err
	}

	user, err := as.users.FindByLogin(ctx, identifier)
	if err != nil {
		if errors.Is(err, apperrors.ErrInvalidCredentials) {
			return nil, "", "", apperrors.ErrInvalidCredentials
		}

		return nil, "", "", fmt.Errorf("find login user: %w", err)
	}

	if err := utils.CheckPassword(user.PasswordHash, password); err != nil {
		return nil, "", "", apperrors.ErrInvalidCredentials
	}

	accessToken, err := as.tokens.Generate(*user)
	if err != nil {
		return nil, "", "", fmt.Errorf("generate auth token: %w", err)
	}

	refreshToken, err := utils.NewOpaqueToken()
	if err != nil {
		return nil, "", "", fmt.Errorf("generate refresh token: %w", err)
	}

	if err := as.sessions.Create(ctx, newRefreshSession(
		user.ID,
		utils.HashToken(refreshToken),
	)); err != nil {
		return nil, "", "", fmt.Errorf("create session: %w", err)
	}

	return user, accessToken, refreshToken, nil
}

func (as *AuthService) Refresh(
	ctx context.Context,
	refreshToken string,
) (string, string, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return "", "", apperrors.ErrSessionNotFound
	}

	session, err := as.sessions.GetByTokenHash(
		ctx,
		utils.HashToken(refreshToken),
	)
	if err != nil {
		return "", "", err
	}

	if session.RevokedAt != nil {
		return "", "", apperrors.ErrSessionRevoked
	}

	if time.Now().UTC().After(session.ExpiresAt) {
		return "", "", apperrors.ErrSessionExpired
	}

	user, err := as.users.GetByID(ctx, session.UserID)
	if err != nil {
		return "", "", fmt.Errorf("get session user: %w", err)
	}

	accessToken, err := as.tokens.Generate(*user)
	if err != nil {
		return "", "", fmt.Errorf("generate access token: %w", err)
	}

	nextRefreshToken, err := utils.NewOpaqueToken()
	if err != nil {
		return "", "", fmt.Errorf("generate refresh token: %w", err)
	}

	nextSession := newRefreshSession(
		user.ID,
		utils.HashToken(nextRefreshToken),
	)
	if err := as.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := as.sessions.Create(txCtx, nextSession); err != nil {
			return fmt.Errorf("create rotated session: %w", err)
		}
		if err := as.sessions.RevokeByID(txCtx, session.ID); err != nil {
			return fmt.Errorf("revoke rotated session: %w", err)
		}
		return nil
	}); err != nil {
		return "", "", fmt.Errorf("rotate session: %w", err)
	}

	return accessToken, nextRefreshToken, nil
}

func (as *AuthService) Logout(ctx context.Context, refreshToken string) error {
	if strings.TrimSpace(refreshToken) == "" {
		return apperrors.ErrSessionNotFound
	}

	session, err := as.sessions.GetByTokenHash(
		ctx,
		utils.HashToken(refreshToken),
	)
	if err != nil {
		return err
	}

	if session.RevokedAt != nil {
		return nil
	}

	if err := as.sessions.RevokeByID(ctx, session.ID); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}

	return nil
}

func loginIdentifier(email, mobileNo *string) (string, error) {
	hasEmail := email != nil && strings.TrimSpace(*email) != ""
	hasMobileNo := mobileNo != nil && strings.TrimSpace(*mobileNo) != ""

	if hasEmail == hasMobileNo {
		return "", apperrors.ErrLoginIdentifier
	}

	if hasEmail {
		return strings.ToLower(strings.TrimSpace(*email)), nil
	}

	return strings.TrimSpace(*mobileNo), nil
}

func newRefreshSession(userID uint64, tokenHash string) *models.Session {
	return &models.Session{
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().UTC().Add(constant.RefreshTokenTTL),
	}
}
