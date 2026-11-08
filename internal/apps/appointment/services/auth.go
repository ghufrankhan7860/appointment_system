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
	parser   UserAgentParser
	logger   Logger
}

// TokenGenerator issues access tokens. The session ID is carried in the token so
// requests know which session (device) they come from.
type TokenGenerator interface {
	Generate(user models.User, sessionID uint64) (string, error)
}

func NewAuthService(
	users UserRepository,
	sessions SessionRepository,
	tokens TokenGenerator,
	parser UserAgentParser,
	logger Logger,
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
	if parser == nil {
		return nil, apperrors.ErrNilUserAgentParser
	}
	if logger == nil {
		return nil, apperrors.ErrNilLogger
	}

	return &AuthService{
		users:    users,
		sessions: sessions,
		tokens:   tokens,
		parser:   parser,
		logger:   logger,
	}, nil
}

func (as *AuthService) Register(
	ctx context.Context,
	user *models.User,
	password string,
) (*models.User, error) {
	if len(password) < constant.MinPasswordLength {
		return nil, apperrors.Invalid(apperrors.ErrInvalidRegisterInput, "password", fmt.Sprintf("must be at least %d characters", constant.MinPasswordLength))
	}

	passwordHash, err := utils.HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("error hashing password: %w", err)
	}

	user.PasswordHash = passwordHash

	createdUser, err := as.users.Create(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", duplicateUserReason(err))
	}
	as.logger.InfoContext(ctx, "user registered", "user_id", createdUser.ID)

	return createdUser, nil
}

// Login checks the credentials and opens a session recording the client: its IP,
// User-Agent (parsed), language and the device headers the app sent. The access
// token carries the new session's ID.
func (as *AuthService) Login(
	ctx context.Context,
	email *string,
	mobileNo *string,
	password string,
	client models.SessionClient,
) (*models.User, string, string, error) {
	identifier, isEmail, err := loginIdentifier(email, mobileNo)
	if err != nil {
		return nil, "", "", err
	}
	client, err = validateSessionClient(client)
	if err != nil {
		return nil, "", "", err
	}

	var user *models.User
	if isEmail {
		user, err = as.users.FindByEmail(ctx, identifier)
	} else {
		user, err = as.users.FindByMobile(ctx, identifier)
	}
	if err != nil {
		if errors.Is(err, apperrors.ErrInvalidCredentials) {
			return nil, "", "", apperrors.ErrInvalidCredentials
		}

		return nil, "", "", fmt.Errorf("find login user: %w", err)
	}

	if err := utils.CheckPassword(user.PasswordHash, password); err != nil {
		as.logger.WarnContext(ctx, "login failed", "user_id", user.ID)
		return nil, "", "", apperrors.ErrInvalidCredentials
	}

	refreshToken, err := utils.NewOpaqueToken()
	if err != nil {
		return nil, "", "", fmt.Errorf("generate refresh token: %w", err)
	}

	session := newRefreshSession(user.ID, utils.HashToken(refreshToken))
	session.IPAddress = client.IPAddress
	session.UserAgent = client.UserAgent
	session.Browser = as.parser.Parse(client.UserAgent)
	session.DeviceID = client.DeviceID
	session.DeviceName = client.DeviceName
	session.DevicePlatform = client.DevicePlatform
	session.AppVersion = client.AppVersion
	session.Screen = client.Screen
	session.Timezone = client.Timezone
	session.Language = client.AcceptLanguage
	if err := as.sessions.Create(ctx, session); err != nil {
		return nil, "", "", fmt.Errorf("create session: %w", err)
	}

	accessToken, err := as.tokens.Generate(*user, session.ID)
	if err != nil {
		return nil, "", "", fmt.Errorf("generate auth token: %w", err)
	}
	as.logger.InfoContext(ctx, "user logged in", "user_id", user.ID, "role", user.Role, "session_id", session.ID)

	return user, accessToken, refreshToken, nil
}

// Refresh rotates the session's refresh token in place, so one login stays one
// session row, and records where the client is now: IP, User-Agent, app version
// and timezone. The login IP and device ID never change. A device ID that differs
// from the one recorded at login is allowed but logged, since it can mean the
// refresh token is used from another device.
func (as *AuthService) Refresh(
	ctx context.Context,
	refreshToken string,
	client models.SessionClient,
) (string, string, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return "", "", apperrors.ErrSessionNotFound
	}
	client, err := validateSessionClient(client)
	if err != nil {
		return "", "", err
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
	if session.DeviceID != "" && client.DeviceID != "" && session.DeviceID != client.DeviceID {
		as.logger.WarnContext(ctx, "refresh device mismatch", "session_id", session.ID, "user_id", user.ID)
	}

	nextRefreshToken, err := utils.NewOpaqueToken()
	if err != nil {
		return "", "", fmt.Errorf("generate refresh token: %w", err)
	}

	oldTokenHash := session.TokenHash
	session.TokenHash = utils.HashToken(nextRefreshToken)
	session.ExpiresAt = time.Now().UTC().Add(constant.RefreshTokenTTL)
	session.LastSeenIP = client.IPAddress
	session.UserAgent = client.UserAgent
	session.Browser = as.parser.Parse(client.UserAgent)
	if client.AppVersion != "" {
		session.AppVersion = client.AppVersion
	}
	if client.Timezone != "" {
		session.Timezone = client.Timezone
	}
	if err := as.sessions.Rotate(ctx, session.ID, oldTokenHash, session); err != nil {
		if errors.Is(err, apperrors.ErrSessionNotFound) {
			return "", "", err
		}
		return "", "", fmt.Errorf("rotate session: %w", err)
	}

	accessToken, err := as.tokens.Generate(*user, session.ID)
	if err != nil {
		return "", "", fmt.Errorf("generate access token: %w", err)
	}
	as.logger.InfoContext(ctx, "session refreshed", "session_id", session.ID, "user_id", user.ID)

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
	as.logger.InfoContext(ctx, "session revoked", "session_id", session.ID, "user_id", session.UserID)

	return nil
}

// loginIdentifier returns the one identifier given, normalised, and whether
// it is an email.
func loginIdentifier(email, mobileNo *string) (string, bool, error) {
	hasEmail := email != nil && strings.TrimSpace(*email) != ""
	hasMobileNo := mobileNo != nil && strings.TrimSpace(*mobileNo) != ""

	if hasEmail == hasMobileNo {
		return "", false, apperrors.ErrLoginIdentifier
	}

	if hasEmail {
		return strings.ToLower(strings.TrimSpace(*email)), true, nil
	}

	return strings.TrimSpace(*mobileNo), false, nil
}

func newRefreshSession(userID uint64, tokenHash string) *models.Session {
	return &models.Session{
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().UTC().Add(constant.RefreshTokenTTL),
	}
}

func duplicateUserReason(err error) error {
	switch {
	case errors.Is(err, apperrors.ErrEmailAlreadyExists):
		return apperrors.Invalid(apperrors.ErrEmailAlreadyExists, "email", "is already registered")
	case errors.Is(err, apperrors.ErrMobileAlreadyExists):
		return apperrors.Invalid(apperrors.ErrMobileAlreadyExists, "mobileNo", "is already registered")
	default:
		return err
	}
}
