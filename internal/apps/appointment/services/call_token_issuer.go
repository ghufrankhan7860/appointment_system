package services

import (
	"appointment_system/internal/apps/appointment/models"
	"time"
)

// CallTokenIssuer signs and checks the tokens that open the call signaling
// WebSocket (utils.CallTokenManager).
type CallTokenIssuer interface {
	Generate(claims models.CallTokenClaims) (string, time.Time, error)
	Verify(token string) (models.CallTokenClaims, error)
}
