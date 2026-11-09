package call

type JoinCallRoomRequest struct {
	Code        string `json:"-" uri:"code" swaggerignore:"true"`
	DisplayName string `json:"displayName" binding:"required,max=100" example:"Ravi Rao"`
	MediaMode   string `json:"mediaMode" binding:"omitempty,oneof=audio video" enums:"audio,video" example:"audio"`
}
