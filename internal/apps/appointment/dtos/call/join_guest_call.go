package call

type JoinGuestCallRequest struct {
	Token       string `json:"-" uri:"token" swaggerignore:"true"`
	DisplayName string `json:"displayName" binding:"omitempty,max=100" example:"Ravi Rao"`
	MediaMode   string `json:"mediaMode" binding:"omitempty,oneof=audio video" enums:"audio,video" example:"audio"`
}
