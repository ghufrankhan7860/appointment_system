package desk

import "time"

// CreateClaimCodeRequest asks for a code with which a walk-in, or whoever
// looks after them (relationship), takes the patient over in the app.
type CreateClaimCodeRequest struct {
	ID           uint64 `json:"-" uri:"id" swaggerignore:"true"`
	Relationship string `json:"relationship" binding:"required" enums:"self,father,mother,spouse,child,sibling,guardian,caregiver,other" example:"self"`
}

// ClaimCodeResponse is redeemed at /claim in the app, once, before ExpiresAt.
type ClaimCodeResponse struct {
	Code         string    `json:"code" example:"K7Q2M9XD"`
	Relationship string    `json:"relationship" example:"self"`
	ExpiresAt    time.Time `json:"expiresAt"`
}
