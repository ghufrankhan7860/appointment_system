package call

import "encoding/json"

type CallSocketRequest struct {
	Token string `form:"token" binding:"required"`
}

// CallSignalMessage is one JSON message on the signaling WebSocket, in both
// directions. Only the fields its type uses are set.
type CallSignalMessage struct {
	Type      string                  `json:"type"`
	To        string                  `json:"to,omitempty"`
	From      string                  `json:"from,omitempty"`
	SDP       string                  `json:"sdp,omitempty"`
	Candidate json.RawMessage         `json:"candidate,omitempty"`
	Mode      string                  `json:"mode,omitempty"`
	State     string                  `json:"state,omitempty"`
	Action    string                  `json:"action,omitempty"`
	PeerID    string                  `json:"peerId,omitempty"`
	Peer      *CallSignalPeerMessage  `json:"peer,omitempty"`
	Peers     []CallSignalPeerMessage `json:"peers,omitempty"`
	Reason    string                  `json:"reason,omitempty"`
	Text      string                  `json:"text,omitempty"`
	MessageID string                  `json:"messageId,omitempty"`
	Emoji     string                  `json:"emoji,omitempty"`
	Reacted   bool                    `json:"reacted,omitempty"`
	ChatOff   bool                    `json:"chatOff,omitempty"`
}

type CallSignalPeerMessage struct {
	PeerID      string `json:"peerId"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
	MediaMode   string `json:"mediaMode"`
	MutedByHost bool   `json:"mutedByHost"`
}
