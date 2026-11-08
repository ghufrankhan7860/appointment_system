package admin

import (
	"appointment_system/internal/apps/appointment/dtos/auth"
	"time"
)

type ListUserSessionsRequest struct {
	ID     uint64 `uri:"id" form:"-"`
	Limit  int    `uri:"-" form:"limit"`
	Offset int    `uri:"-" form:"offset"`
}

// UserSessionResponse is a session as an admin sees it: in any status, with the
// raw User-Agent and device ID kept for investigations.
type UserSessionResponse struct {
	auth.SessionResponse
	Status    string     `json:"status" enums:"active,revoked,expired" example:"active"`
	RevokedAt *time.Time `json:"revokedAt,omitempty"`
	UserAgent string     `json:"userAgent"`
	DeviceID  string     `json:"deviceId"`
}

type ListUserSessionsResponse struct {
	Sessions []UserSessionResponse `json:"sessions"`
}
