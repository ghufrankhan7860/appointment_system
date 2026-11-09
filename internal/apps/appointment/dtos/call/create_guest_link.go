package call

import "time"

type CreateGuestLinkRequest struct {
	ID          uint64 `json:"-" uri:"id" swaggerignore:"true"`
	DisplayName string `json:"displayName" binding:"omitempty,max=100" example:"Ravi Rao"`
	MaxUses     int    `json:"maxUses" binding:"omitempty,min=1,max=5" example:"3"`
}

// GuestLinkResponse is a guest link. Token is set only in the response that
// created it; the frontend builds the join URL from it.
type GuestLinkResponse struct {
	ID          uint64    `json:"id"`
	Token       string    `json:"token,omitempty"`
	DisplayName string    `json:"displayName,omitempty"`
	MaxUses     int       `json:"maxUses" example:"3"`
	ExpiresAt   time.Time `json:"expiresAt"`
}
