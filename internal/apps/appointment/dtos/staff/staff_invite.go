package staff

import "time"

// StaffInviteResponse is a pending staff invite. Code is set only in the
// response that created it.
type StaffInviteResponse struct {
	ID        uint64    `json:"id"`
	Code      string    `json:"code,omitempty" example:"K7Q2M9XD"`
	ExpiresAt time.Time `json:"expiresAt"`
	CreatedAt time.Time `json:"createdAt"`
}

type ListStaffInvitesResponse struct {
	Invites []StaffInviteResponse `json:"invites"`
}

type RevokeStaffInviteRequest struct {
	ID uint64 `uri:"id" binding:"required"`
}
