package auth

type RevokeSessionRequest struct {
	ID uint64 `uri:"id" binding:"required"`
}
