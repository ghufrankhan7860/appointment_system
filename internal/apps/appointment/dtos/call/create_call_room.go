package call

import "time"

type CreateCallRoomRequest struct {
	Title            string     `json:"title" binding:"omitempty,max=100" example:"Family briefing"`
	MaxParticipants  int        `json:"maxParticipants" binding:"omitempty,min=2,max=5" example:"5"`
	RequiresApproval *bool      `json:"requiresApproval" example:"true"`
	ScheduledAt      *time.Time `json:"scheduledAt"`
}

// CallRoomResponse is a code room. Code is set only in the response that
// created it.
type CallRoomResponse struct {
	ID               uint64     `json:"id"`
	Code             string     `json:"code,omitempty" example:"4829137"`
	Title            string     `json:"title" example:"Family briefing"`
	MaxParticipants  int        `json:"maxParticipants" example:"5"`
	RequiresApproval bool       `json:"requiresApproval" example:"true"`
	ScheduledAt      *time.Time `json:"scheduledAt,omitempty"`
	Status           string     `json:"status" enums:"open,active,ended" example:"open"`
	OpenedAt         time.Time  `json:"openedAt"`
	StartedAt        *time.Time `json:"startedAt,omitempty"`
	EndedAt          *time.Time `json:"endedAt,omitempty"`
	EndReason        string     `json:"endReason,omitempty" enums:"hangup,host_ended,timeout,expired" example:"host_ended"`
	ExpiresAt        *time.Time `json:"expiresAt,omitempty"`
}
