package admin

type RevokeUserSessionsRequest struct {
	ID uint64 `uri:"id" binding:"required"`
}

type RevokeUserSessionsResponse struct {
	Revoked int64 `json:"revoked"`
}
