// Package constant holds stable appointment protocol or domain values used across files.
//
// Prefer typed constants next to the type they constrain. Add values here only when they
// are genuinely shared and stable; do not turn this package into a collection of unrelated
// strings, configuration keys, or mutable state.
package constant

const (
	ACCESS_TOKEN_EXPIRATION       = 15 * 60 // 15 minutes in seconds
	REFRESH_TOKEN_EXPIRATION_DAYS = 7       // 7 days
)
