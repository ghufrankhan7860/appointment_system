package patient

type RevokePatientInviteRequest struct {
	ID       uint64 `uri:"id" binding:"required"`
	InviteID uint64 `uri:"inviteId" binding:"required"`
}
