// Package constant holds stable appointment protocol or domain values used across files.
//
// Prefer typed constants next to the type they constrain. Add values here only when they
// are genuinely shared and stable; do not turn this package into a collection of unrelated
// strings, configuration keys, or mutable state.
package constant

import "time"

const (
	AccessTokenTTL  time.Duration = 15 * time.Minute
	RefreshTokenTTL time.Duration = 7 * 24 * time.Hour
)
