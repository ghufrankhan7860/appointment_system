package admin

type RevokeUserSessionRequest struct {
	ID        uint64 `uri:"id" binding:"required"`
	SessionID uint64 `uri:"sessionId" binding:"required"`
}
