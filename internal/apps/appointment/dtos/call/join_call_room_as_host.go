package call

type JoinCallRoomAsHostRequest struct {
	ID        uint64 `json:"-" uri:"id" swaggerignore:"true"`
	MediaMode string `json:"mediaMode" binding:"omitempty,oneof=audio video" enums:"audio,video" example:"audio"`
}
