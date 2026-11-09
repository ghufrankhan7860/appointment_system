package call

type EndCallRequest struct {
	ID uint64 `uri:"id" binding:"required"`
}
