package patient

type RevokePatientMemberRequest struct {
	ID     uint64 `uri:"id" binding:"required"`
	UserID uint64 `uri:"userId" binding:"required"`
}
