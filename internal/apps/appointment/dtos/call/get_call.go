package call

import "time"

type GetCallRequest struct {
	ID uint64 `uri:"id" binding:"required"`
}

type GetCallResponse struct {
	RoomID       uint64                    `json:"roomId"`
	Status       string                    `json:"status" enums:"open,active" example:"active"`
	OpenedAt     time.Time                 `json:"openedAt"`
	StartedAt    *time.Time                `json:"startedAt,omitempty"`
	Participants []CallParticipantResponse `json:"participants"`
}

// CallParticipantResponse is one person who joined a room. InRoom marks the
// ones connected right now.
type CallParticipantResponse struct {
	PeerID          string `json:"peerId" example:"3f9c2a7d1b6e8f40"`
	DisplayName     string `json:"displayName" example:"Asha Rao"`
	Role            string `json:"role" enums:"host,doctor,patient_side,guest,member" example:"patient_side"`
	AdmissionStatus string `json:"admissionStatus" enums:"admitted,waiting,denied,removed" example:"admitted"`
	MediaMode       string `json:"mediaMode" enums:"audio,video" example:"audio"`
	MutedByHost     bool   `json:"mutedByHost"`
	InRoom          bool   `json:"inRoom"`
}
