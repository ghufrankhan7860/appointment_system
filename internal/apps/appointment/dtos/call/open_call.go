package call

import "time"

type OpenCallRequest struct {
	ID        uint64 `json:"-" uri:"id" swaggerignore:"true"`
	MediaMode string `json:"mediaMode" binding:"omitempty,oneof=audio video" enums:"audio,video" example:"audio"`
}

// CallJoinResponse is what a participant needs to connect: wsToken opens GET
// /calls/ws, and iceServers go to the RTCPeerConnection. Waiting is set while
// a code room's host has not admitted the participant yet.
type CallJoinResponse struct {
	RoomID           uint64              `json:"roomId"`
	PeerID           string              `json:"peerId" example:"3f9c2a7d1b6e8f40"`
	Role             string              `json:"role" enums:"host,doctor,patient_side,guest,member" example:"doctor"`
	Waiting          bool                `json:"waiting"`
	WSToken          string              `json:"wsToken"`
	WSTokenExpiresAt time.Time           `json:"wsTokenExpiresAt"`
	ICEServers       []ICEServerResponse `json:"iceServers"`
}

type ICEServerResponse struct {
	URLs       []string `json:"urls" example:"stun:stun.l.google.com:19302"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}
