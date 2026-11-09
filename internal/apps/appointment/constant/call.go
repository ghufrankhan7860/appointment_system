package constant

import "time"

// Rules for audio and video calls.
const (
	// CallJoinLeadTime and CallJoinGrace frame when an appointment's call can
	// be joined: from CallJoinLeadTime before its start until CallJoinGrace
	// after its end. A scheduled code room opens CallJoinLeadTime early too.
	CallJoinLeadTime = 10 * time.Minute
	CallJoinGrace    = 30 * time.Minute
	// AppointmentCallMaxParticipants is the doctor and one patient-side peer.
	AppointmentCallMaxParticipants = 2
	// MaxCallRoomOpenTries lets a request that lost the race to create an
	// appointment's room join the room the winner created.
	MaxCallRoomOpenTries = 2
	// CallRoomListLimit is how many of a host's code rooms are listed, newest
	// first.
	CallRoomListLimit = 50
	// MinCodeRoomParticipants and MaxCodeRoomParticipants bound a code room.
	// Calls are peer to peer, so every peer uploads to every other one; more
	// than five does not fit mobile upload bandwidth.
	MinCodeRoomParticipants = 2
	MaxCodeRoomParticipants = 5
	// CodeRoomTTL is how long an unscheduled code room lives; a scheduled one
	// lives until CodeRoomScheduledTTL after its scheduled time.
	CodeRoomTTL          = 24 * time.Hour
	CodeRoomScheduledTTL = 2 * time.Hour
	// MaxCodeRoomScheduleAhead bounds how far ahead a code room can be
	// scheduled.
	MaxCodeRoomScheduleAhead = 30 * 24 * time.Hour
	// MaxLiveCodeRoomsPerHost and MaxCodeRoomsPerDay cap the code rooms one
	// doctor holds open and creates in a rolling day.
	MaxLiveCodeRoomsPerHost = 3
	MaxCodeRoomsPerDay      = 20
	CodeRoomDailyWindow     = 24 * time.Hour
	// CodeRoomCodeDigits random digits from CodeRoomCodeAlphabet plus one
	// check digit make a join code.
	CodeRoomCodeAlphabet = "0123456789"
	CodeRoomCodeDigits   = 6
	// MaxCodeRoomCodeTries bounds new codes drawn when one clashes with the code
	// of a live room.
	MaxCodeRoomCodeTries = 3
	// CodeRoomHostGrace is how long a code room outlives its host leaving.
	CodeRoomHostGrace = 5 * time.Minute
	// CodeRoomIdleTimeout ends a code room nobody admitted has been in for
	// this long.
	CodeRoomIdleTimeout = 30 * time.Minute
	// DefaultGuestLinkUses and MaxGuestLinkUses bound how many times one guest
	// link can be used to join.
	DefaultGuestLinkUses = 3
	MaxGuestLinkUses     = 5
	// MaxFailedCallJoinAttempts wrong codes or guest links from one IP within
	// CallJoinAttemptWindow block further joins from it until the window passes.
	MaxFailedCallJoinAttempts = 5
	CallJoinAttemptWindow     = 10 * time.Minute
	// CallTokenTTL is how long a call token can open the signaling WebSocket.
	CallTokenTTL = 15 * time.Minute
	// TURNCredentialTTL is how long the TURN credentials handed to a peer stay
	// valid.
	TURNCredentialTTL = time.Hour
	// CallHeartbeatInterval is how often the server pings a socket; a peer not
	// seen for CallPeerTimeout is out of the room.
	CallHeartbeatInterval = 20 * time.Second
	CallPeerTimeout       = 60 * time.Second
	// CallSignalWriteTimeout bounds one write to a socket.
	CallSignalWriteTimeout = 10 * time.Second
	// MaxCallSignalBytes bounds one signaling message; an SDP offer is a few KB.
	MaxCallSignalBytes = 64 * 1024
	// CallSignalBuffer is how many messages wait for a slow socket before it is
	// dropped.
	CallSignalBuffer = 32
	// CallPeerIDBytes random bytes, hex encoded, make a peer id.
	CallPeerIDBytes = 8
	// MaxCallDisplayNameLength and MaxCallTitleLength bound the free text of a
	// call.
	MaxCallDisplayNameLength = 100
	MaxCallTitleLength       = 100
)
