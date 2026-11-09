package models

import (
	"encoding/json"
	"time"
)

type CallRoomKind string

const (
	CallRoomKindAppointment CallRoomKind = "appointment"
	CallRoomKindCode        CallRoomKind = "code"
)

type CallRoomStatus string

const (
	CallRoomStatusOpen   CallRoomStatus = "open"
	CallRoomStatusActive CallRoomStatus = "active"
	CallRoomStatusEnded  CallRoomStatus = "ended"
)

type CallEndReason string

const (
	CallEndReasonHangup            CallEndReason = "hangup"
	CallEndReasonHostEnded         CallEndReason = "host_ended"
	CallEndReasonTimeout           CallEndReason = "timeout"
	CallEndReasonAppointmentClosed CallEndReason = "appointment_closed"
	CallEndReasonNoAnswer          CallEndReason = "no_answer"
	CallEndReasonExpired           CallEndReason = "expired"
)

type CallParticipantRole string

const (
	CallRoleHost        CallParticipantRole = "host"
	CallRoleDoctor      CallParticipantRole = "doctor"
	CallRolePatientSide CallParticipantRole = "patient_side"
	CallRoleGuest       CallParticipantRole = "guest"
	CallRoleMember      CallParticipantRole = "member"
)

type CallAdmissionStatus string

const (
	CallAdmissionAdmitted CallAdmissionStatus = "admitted"
	CallAdmissionWaiting  CallAdmissionStatus = "waiting"
	CallAdmissionDenied   CallAdmissionStatus = "denied"
	CallAdmissionRemoved  CallAdmissionStatus = "removed"
)

type CallMediaMode string

const (
	CallMediaAudio CallMediaMode = "audio"
	CallMediaVideo CallMediaMode = "video"
)

// CallConnectionState is the last ICE state a participant reported. Relay
// means connected through TURN.
type CallConnectionState string

const (
	CallConnectionConnected CallConnectionState = "connected"
	CallConnectionRelay     CallConnectionState = "relay"
	CallConnectionFailed    CallConnectionState = "failed"
)

type CallEventType string

const (
	CallEventOpened          CallEventType = "opened"
	CallEventJoined          CallEventType = "joined"
	CallEventLeft            CallEventType = "left"
	CallEventAdmitted        CallEventType = "admitted"
	CallEventDenied          CallEventType = "denied"
	CallEventRemoved         CallEventType = "removed"
	CallEventMuted           CallEventType = "muted"
	CallEventUnmuted         CallEventType = "unmuted"
	CallEventMediaChanged    CallEventType = "media_changed"
	CallEventConnectionState CallEventType = "connection_state"
	CallEventEnded           CallEventType = "ended"
)

// CallRoom is one call. An appointment room belongs to an appointment and
// holds its doctor and one patient-side peer; a code room is hosted by a doctor
// and joined with a code, of which only the sha256 is kept. An appointment has
// at most one open or active room.
type CallRoom struct {
	ID               uint64
	Kind             CallRoomKind
	AppointmentID    *uint64
	HostUserID       uint64
	Title            string
	JoinCodeHash     string
	MaxParticipants  int
	RequiresApproval bool
	ScheduledAt      *time.Time
	Status           CallRoomStatus
	OpenedAt         time.Time
	StartedAt        *time.Time
	EndedAt          *time.Time
	EndReason        CallEndReason
	ExpiresAt        *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// CallParticipant is one person in a room. UserID is nil for a guest or a
// code-room joiner, who are known only by DisplayName. PeerID addresses the
// participant in signaling messages. A participant is in the room while
// LeftAt is nil and LastSeenAt is recent.
type CallParticipant struct {
	ID              uint64
	CallRoomID      uint64
	UserID          *uint64
	GuestTokenID    *uint64
	PeerID          string
	DisplayName     string
	Role            CallParticipantRole
	AdmissionStatus CallAdmissionStatus
	MediaMode       CallMediaMode
	MutedByHost     bool
	ConnectionState CallConnectionState
	JoinedAt        *time.Time
	LastSeenAt      *time.Time
	LeftAt          *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// CallGuestToken lets someone without an account join an appointment's call
// as the patient side, up to MaxUses times. Only the sha256 of the token is
// kept.
type CallGuestToken struct {
	ID              uint64
	AppointmentID   uint64
	CreatedByUserID uint64
	TokenHash       string
	DisplayName     string
	ExpiresAt       time.Time
	MaxUses         int
	UseCount        int
	RevokedAt       *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type CallEvent struct {
	ID                uint64
	CallRoomID        uint64
	CallParticipantID *uint64
	EventType         CallEventType
	Detail            string
	CreatedAt         time.Time
}

// CallRoomState is a room with its participants. InRoom marks the ones
// connected right now.
type CallRoomState struct {
	Room         CallRoom
	Participants []CallPresence
}

type CallPresence struct {
	Participant CallParticipant
	InRoom      bool
}

type ICEServer struct {
	URLs       []string
	Username   string
	Credential string
}

// CallJoin is what a participant needs to connect: the signaling token for
// the WebSocket and the ICE servers for the peer connections.
type CallJoin struct {
	Room           CallRoom
	Participant    CallParticipant
	Token          string
	TokenExpiresAt time.Time
	ICEServers     []ICEServer
}

// CallTokenClaims are carried by a call token, which opens the signaling
// WebSocket for one participant of one room.
type CallTokenClaims struct {
	RoomID        uint64
	ParticipantID uint64
	PeerID        string
	Role          CallParticipantRole
}

type CallSignalType string

// Signaling messages. Offer, answer and ice go to the peer in To and arrive
// with From; the rest are sent by the server or broadcast to the room.
const (
	CallSignalWelcome    CallSignalType = "welcome"
	CallSignalPeerJoined CallSignalType = "peer-joined"
	CallSignalPeerLeft   CallSignalType = "peer-left"
	CallSignalLobby      CallSignalType = "lobby"
	CallSignalOffer      CallSignalType = "offer"
	CallSignalAnswer     CallSignalType = "answer"
	CallSignalICE        CallSignalType = "ice"
	CallSignalMedia      CallSignalType = "media"
	CallSignalState      CallSignalType = "state"
	CallSignalLeave      CallSignalType = "leave"
	CallSignalHost       CallSignalType = "host"
	CallSignalEnded      CallSignalType = "ended"
	CallSignalError      CallSignalType = "error"
)

type CallHostAction string

const (
	CallHostAdmit  CallHostAction = "admit"
	CallHostDeny   CallHostAction = "deny"
	CallHostRemove CallHostAction = "remove"
	CallHostMute   CallHostAction = "mute"
	CallHostUnmute CallHostAction = "unmute"
	CallHostEnd    CallHostAction = "end"
)

// CallSignal is one signaling message. Only the fields its Type uses are set.
type CallSignal struct {
	Type      CallSignalType
	To        string
	From      string
	SDP       string
	Candidate json.RawMessage
	Mode      CallMediaMode
	State     CallConnectionState
	Action    CallHostAction
	PeerID    string
	Peer      *CallSignalPeer
	Peers     []CallSignalPeer
	Reason    string
}

// CallSignalPeer describes a participant to the others in the room.
type CallSignalPeer struct {
	PeerID      string
	DisplayName string
	Role        CallParticipantRole
	MediaMode   CallMediaMode
	MutedByHost bool
}
