// Package constant is the one place for the application's fixed values: formats,
// limits, defaults and token lifetimes. Each topic has its own file (auth.go,
// time.go, doctor.go, availability.go, pagination.go). Code uses these names
// instead of literal numbers or strings. Enum values that belong to a type stay
// next to that type in models, and environment configuration stays in config.
package constant

import "time"

const (
	AccessTokenTTL  time.Duration = 15 * time.Minute
	RefreshTokenTTL time.Duration = 7 * 24 * time.Hour

	// MinJWTSecretLength is the shortest HS256 signing secret accepted at startup.
	MinJWTSecretLength = 32
	// MinPasswordLength is the shortest password a new patient or doctor account
	// may set. Older accounts keep signing in with shorter ones.
	MinPasswordLength = 8
	// MinAdminPasswordLength is stricter than the public registration minimum.
	MinAdminPasswordLength = 12
	// RefreshTokenBytes is the random size of an opaque refresh token (hex-encoded to twice this).
	RefreshTokenBytes = 32

	// SessionRetention is how long a session stays in the table after it expired or was
	// revoked; it holds the client IP and device details, which are personal data.
	SessionRetention time.Duration = 90 * 24 * time.Hour
	// SessionPurgeBatchSize is how many ended sessions one purge run deletes per query.
	SessionPurgeBatchSize = 500
	// MaxUserAgentLength caps the stored User-Agent; a longer one is cut, never refused.
	MaxUserAgentLength = 512
	// MaxLanguageLength caps the first Accept-Language tag (BCP 47 tags fit in 35).
	MaxLanguageLength = 35
	// Device header limits; they match the sessions columns.
	MaxDeviceIDLength   = 64
	MaxDeviceNameLength = 100
	MaxAppVersionLength = 30
	// UserAgentCacheSize is how many distinct User-Agents the parser keeps parsed in memory.
	UserAgentCacheSize = 1000
)
