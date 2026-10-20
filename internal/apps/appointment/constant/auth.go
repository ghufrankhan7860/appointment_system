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
	// MinAdminPasswordLength is stricter than the public registration minimum.
	MinAdminPasswordLength = 12
	// RefreshTokenBytes is the random size of an opaque refresh token (hex-encoded to twice this).
	RefreshTokenBytes = 32
)
