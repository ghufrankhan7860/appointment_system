package patient

import "time"

type CreatePatientInviteRequest struct {
	ID             uint64 `json:"-" uri:"id" swaggerignore:"true"`
	Relationship   string `json:"relationship" binding:"required" enums:"self,father,mother,spouse,child,sibling,guardian,caregiver,other" example:"mother"`
	AccessLevel    string `json:"accessLevel" binding:"required" enums:"full,appointments_only,read_only" example:"full"`
	ExpiresInHours int    `json:"expiresInHours" binding:"omitempty,min=1,max=168" example:"48"`
}

// PatientInviteResponse is a pending invite. Code is set only in the response
// that created it.
type PatientInviteResponse struct {
	ID           uint64    `json:"id"`
	Code         string    `json:"code,omitempty" example:"K7Q2M9XD"`
	Relationship string    `json:"relationship" example:"mother"`
	AccessLevel  string    `json:"accessLevel" example:"full"`
	ExpiresAt    time.Time `json:"expiresAt"`
	CreatedAt    time.Time `json:"createdAt"`
}
