package call

type ListCallRoomParticipantsRequest struct {
	ID uint64 `uri:"id" binding:"required"`
}

type ListCallRoomParticipantsResponse struct {
	Participants []CallParticipantResponse `json:"participants"`
}
