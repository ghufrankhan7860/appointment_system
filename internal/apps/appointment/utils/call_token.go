package utils

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
)

const (
	callTokenAudience = "call"
	callTokenKeyLabel = "call-token"
)

// CallTokenManager issues the tokens that open the call signaling WebSocket.
// They are signed with a key derived from the JWT secret and carry their own
// audience, so a call token is never accepted as an access token and an access
// token never opens a call.
type CallTokenManager struct {
	secret []byte
	issuer string
	ttl    time.Duration
}

type callClaims struct {
	RoomID uint64 `json:"room_id"`
	PeerID string `json:"peer_id"`
	Role   string `json:"participant_role"`
	jwt.RegisteredClaims
}

func NewCallTokenManager(jwtSecret, issuer string, ttl time.Duration) (*CallTokenManager, error) {
	if len(jwtSecret) < constant.MinJWTSecretLength {
		return nil, fmt.Errorf("JWT secret must contain at least %d characters", constant.MinJWTSecretLength)
	}
	if issuer == "" {
		return nil, errors.New("JWT issuer is required")
	}
	if ttl <= 0 {
		return nil, errors.New("call token TTL must be greater than zero")
	}

	mac := hmac.New(sha256.New, []byte(jwtSecret))
	mac.Write([]byte(callTokenKeyLabel))

	return &CallTokenManager{
		secret: mac.Sum(nil),
		issuer: issuer,
		ttl:    ttl,
	}, nil
}

// Generate issues a call token for one participant and returns it with its
// expiry.
func (manager *CallTokenManager) Generate(claims models.CallTokenClaims) (string, time.Time, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(manager.ttl)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, callClaims{
		RoomID: claims.RoomID,
		PeerID: claims.PeerID,
		Role:   string(claims.Role),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    manager.issuer,
			Subject:   strconv.FormatUint(claims.ParticipantID, 10),
			Audience:  jwt.ClaimStrings{callTokenAudience},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	})

	signed, err := token.SignedString(manager.secret)
	if err != nil {
		return "", time.Time{}, err
	}

	return signed, expiresAt, nil
}

func (manager *CallTokenManager) Verify(tokenString string) (models.CallTokenClaims, error) {
	var claims callClaims

	if _, err := jwt.ParseWithClaims(
		tokenString,
		&claims,
		func(*jwt.Token) (any, error) {
			return manager.secret, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(manager.issuer),
		jwt.WithAudience(callTokenAudience),
		jwt.WithExpirationRequired(),
	); err != nil {
		return models.CallTokenClaims{}, err
	}

	participantID, err := strconv.ParseUint(claims.Subject, 10, 64)
	if err != nil || participantID == 0 || claims.RoomID == 0 || claims.PeerID == "" || claims.Role == "" {
		return models.CallTokenClaims{}, errors.New("invalid call token claims")
	}

	return models.CallTokenClaims{
		RoomID:        claims.RoomID,
		ParticipantID: participantID,
		PeerID:        claims.PeerID,
		Role:          models.CallParticipantRole(claims.Role),
	}, nil
}
