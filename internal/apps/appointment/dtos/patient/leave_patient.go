package patient

type LeavePatientRequest struct {
	ID uint64 `uri:"id" binding:"required"`
}
