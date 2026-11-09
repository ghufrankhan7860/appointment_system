package patient

type ListPatientInvitesRequest struct {
	ID uint64 `uri:"id" binding:"required"`
}

type ListPatientInvitesResponse struct {
	Invites []PatientInviteResponse `json:"invites"`
}
