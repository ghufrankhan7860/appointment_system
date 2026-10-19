package utils

import (
	"errors"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"appointment_system/internal/apps/appointment/models"
)

type JWTManager struct {
	secret []byte
	issuer string
	ttl    time.Duration
}

type authClaims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

func NewJWTManager(secret, issuer string, ttl time.Duration) (*JWTManager, error) {
	if len(secret) < 32 {
		return nil, errors.New("JWT secret must contain at least 32 characters")
	}
	if issuer == "" {
		return nil, errors.New("JWT issuer is required")
	}
	if ttl <= 0 {
		return nil, errors.New("JWT TTL must be greater than zero")
	}

	return &JWTManager{
		secret: []byte(secret),
		issuer: issuer,
		ttl:    ttl,
	}, nil
}

func (manager *JWTManager) Generate(user models.User) (string, error) {
	now := time.Now().UTC()
	claims := authClaims{
		Role: string(user.Role),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    manager.issuer,
			Subject:   strconv.FormatUint(user.ID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(manager.ttl)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(manager.secret)
}

func (manager *JWTManager) Verify(tokenString string) (uint64, error) {
	var claims authClaims

	if _, err := jwt.ParseWithClaims(
		tokenString,
		&claims,
		func(*jwt.Token) (any, error) {
			return manager.secret, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(manager.issuer),
		jwt.WithExpirationRequired(),
	); err != nil {
		return 0, err
	}

	userID, err := strconv.ParseUint(claims.Subject, 10, 64)
	if err != nil || userID == 0 {
		return 0, errors.New("invalid token subject")
	}

	return userID, nil
}
