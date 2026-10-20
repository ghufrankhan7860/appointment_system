package doctor

type GetDoctorRequest struct {
	ID uint64 `uri:"id" binding:"required"`
}
