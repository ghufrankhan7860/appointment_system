package call

type EndCallRoomRequest struct {
	ID uint64 `uri:"id" binding:"required"`
}
