package patient

import "time"

type ListPatientMembersRequest struct {
	ID uint64 `uri:"id" binding:"required"`
}

// PatientMemberResponse is one user with access to the patient. Current marks
// the caller.
type PatientMemberResponse struct {
	UserID       uint64    `json:"userId"`
	Name         string    `json:"name"`
	MaskedMobile string    `json:"maskedMobile" example:"98******10"`
	Relationship string    `json:"relationship" example:"mother"`
	AccessLevel  string    `json:"accessLevel" example:"full"`
	Current      bool      `json:"current"`
	Since        time.Time `json:"since"`
}

type ListPatientMembersResponse struct {
	Members []PatientMemberResponse `json:"members"`
}
