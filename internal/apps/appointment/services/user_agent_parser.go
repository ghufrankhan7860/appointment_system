package services

import "appointment_system/internal/apps/appointment/models"

// UserAgentParser reads the browser, operating system and device out of a
// User-Agent header. It is implemented outside this package (adapters/useragent)
// and must be safe for concurrent use. A part it cannot recognise is empty.
type UserAgentParser interface {
	Parse(userAgent string) models.UserAgentDetails
}
