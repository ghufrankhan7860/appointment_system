package call

type RevokeGuestLinkRequest struct {
	ID     uint64 `uri:"id" binding:"required"`
	LinkID uint64 `uri:"linkId" binding:"required"`
}
