package services

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
)

// CallSessionService runs the signaling WebSocket of a call. The server is a
// relay: it passes offers, answers and ICE candidates between peers that are
// both in the room, persists media and connection changes, and applies a code
// room host's lobby and moderation actions. The database is the source of
// truth for who is in a room; the hub only delivers messages. Chat is relayed
// and never stored; a room's chat switch lives in this process's memory.
type CallSessionService struct {
	rooms        CallRoomRepository
	participants CallParticipantRepository
	events       CallEventRepository
	outbox       OutboxRepository
	tx           TransactionManager
	tokens       CallTokenIssuer
	hub          CallHub
	now          func() time.Time
	logger       Logger
	chatMu       sync.Mutex
	chatOff      map[uint64]bool
}

func NewCallSessionService(
	rooms CallRoomRepository,
	participants CallParticipantRepository,
	events CallEventRepository,
	outbox OutboxRepository,
	tx TransactionManager,
	tokens CallTokenIssuer,
	hub CallHub,
	now func() time.Time,
	logger Logger,
) (*CallSessionService, error) {
	if rooms == nil {
		return nil, apperrors.ErrNilCallRoomRepository
	}
	if participants == nil {
		return nil, apperrors.ErrNilCallParticipantRepository
	}
	if events == nil {
		return nil, apperrors.ErrNilCallEventRepository
	}
	if outbox == nil {
		return nil, apperrors.ErrNilOutboxRepository
	}
	if tx == nil {
		return nil, apperrors.ErrNilTransactionManager
	}
	if tokens == nil {
		return nil, apperrors.ErrNilCallTokenIssuer
	}
	if hub == nil {
		return nil, apperrors.ErrNilCallHub
	}
	if now == nil {
		return nil, apperrors.ErrNilClock
	}
	if logger == nil {
		return nil, apperrors.ErrNilLogger
	}

	return &CallSessionService{
		rooms:        rooms,
		participants: participants,
		events:       events,
		outbox:       outbox,
		tx:           tx,
		tokens:       tokens,
		hub:          hub,
		now:          now,
		logger:       logger,
		chatOff:      make(map[uint64]bool),
	}, nil
}

// Authorize checks a call token before its WebSocket is opened: the token is
// valid, its room is live, its participant was not denied or removed, and an
// admitted participant still fits in the room.
func (css *CallSessionService) Authorize(ctx context.Context, token string) (models.CallTokenClaims, error) {
	if err := ctx.Err(); err != nil {
		return models.CallTokenClaims{}, fmt.Errorf("authorize call socket context: %w", err)
	}

	claims, err := css.tokens.Verify(token)
	if err != nil {
		return models.CallTokenClaims{}, apperrors.ErrCallTokenInvalid
	}
	room, participants, self, err := css.load(ctx, claims)
	if err != nil {
		return models.CallTokenClaims{}, err
	}
	if self.AdmissionStatus == models.CallAdmissionAdmitted && !participantFits(room, participants, self, css.now()) {
		return models.CallTokenClaims{}, apperrors.ErrCallRoomFull
	}

	return claims, nil
}

// Connect puts an authorized participant's socket in the room. An admitted
// peer gets a welcome with the peers already in, who are told it joined; the
// newcomer then sends each of them an offer. A waiting peer is told so, and
// the host sees it in the lobby. The second peer in starts the call.
func (css *CallSessionService) Connect(ctx context.Context, claims models.CallTokenClaims, peer CallPeer) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("connect call socket context: %w", err)
	}

	now := css.now()
	var room *models.CallRoom
	var participants []*models.CallParticipant
	var self *models.CallParticipant
	err := css.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		room, err = css.rooms.GetByIDForUpdate(txCtx, claims.RoomID)
		if err != nil {
			return err
		}
		if room.Status == models.CallRoomStatusEnded {
			return apperrors.ErrCallRoomNotFound
		}
		participants, err = css.participants.ListByRoom(txCtx, room.ID)
		if err != nil {
			return fmt.Errorf("list call participants: %w", err)
		}
		self = findCallParticipant(participants, func(candidate *models.CallParticipant) bool {
			return candidate.ID == claims.ParticipantID
		})
		if self == nil || self.PeerID != claims.PeerID || !mayConnect(self) {
			return apperrors.ErrCallTokenInvalid
		}
		admitted := self.AdmissionStatus == models.CallAdmissionAdmitted
		if admitted && !participantFits(room, participants, self, now) {
			return apperrors.ErrCallRoomFull
		}

		if self.JoinedAt == nil {
			self.JoinedAt = &now
		}
		self.LastSeenAt = &now
		self.LeftAt = nil
		if _, err := css.participants.Update(txCtx, self); err != nil {
			return fmt.Errorf("mark call participant connected: %w", err)
		}
		if err := css.events.Create(txCtx, callEvent(room.ID, self.ID, models.CallEventJoined, string(self.MediaMode))); err != nil {
			return fmt.Errorf("create call joined event: %w", err)
		}
		if admitted {
			return css.start(txCtx, room, participants, self, now)
		}

		return nil
	})
	if err != nil {
		if errors.Is(err, apperrors.ErrCallRoomNotFound) ||
			errors.Is(err, apperrors.ErrCallTokenInvalid) ||
			errors.Is(err, apperrors.ErrCallRoomFull) {
			return err
		}
		return fmt.Errorf("connect call socket: %w", err)
	}

	if replaced := css.hub.Join(room.ID, peer); replaced != nil {
		replaced.Close()
	}

	if self.AdmissionStatus == models.CallAdmissionAdmitted {
		css.welcome(room.ID, participants, self, now)
		if self.Role == models.CallRoleHost {
			for _, participant := range participants {
				if participant.AdmissionStatus == models.CallAdmissionWaiting && connected(participant, now) {
					lobbyPeer := signalPeer(participant)
					_ = peer.Send(models.CallSignal{Type: models.CallSignalLobby, Peer: &lobbyPeer})
				}
			}
		}
	} else {
		_ = peer.Send(models.CallSignal{Type: models.CallSignalLobby, PeerID: self.PeerID})
		css.notifyHost(room.ID, participants, models.CallSignal{Type: models.CallSignalLobby, Peer: peerOf(self)}, now)
	}

	css.logger.InfoContext(ctx, "call socket connected", "call_room_id", room.ID, "call_participant_id", self.ID, "admission_status", self.AdmissionStatus)

	return nil
}

// Heartbeat records that the participant is still connected and reports
// whether its room is still live.
func (css *CallSessionService) Heartbeat(ctx context.Context, claims models.CallTokenClaims) (bool, error) {
	if err := css.participants.Touch(ctx, claims.ParticipantID, css.now()); err != nil {
		return false, fmt.Errorf("touch call participant: %w", err)
	}

	room, err := css.rooms.GetByID(ctx, claims.RoomID)
	if err != nil {
		if errors.Is(err, apperrors.ErrCallRoomNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("get call room: %w", err)
	}

	return room.Status != models.CallRoomStatusEnded, nil
}

// Disconnect takes a closed socket out of the room. A socket that was already
// replaced by a newer connection of the same participant, or closed by the
// server, leaves the room as it is.
func (css *CallSessionService) Disconnect(ctx context.Context, claims models.CallTokenClaims, peer CallPeer) error {
	if !css.hub.Leave(claims.RoomID, peer) {
		return nil
	}

	return css.leave(ctx, claims)
}

// Handle applies one message from a participant's socket. A message the
// participant may not send is answered with an error message; the returned
// error is only for failures the socket cannot recover from.
func (css *CallSessionService) Handle(ctx context.Context, claims models.CallTokenClaims, signal models.CallSignal) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("handle call signal context: %w", err)
	}

	now := css.now()
	room, participants, self, err := css.load(ctx, claims)
	if errors.Is(err, apperrors.ErrCallRoomNotFound) {
		css.hub.ClosePeer(claims.RoomID, claims.PeerID, models.CallSignal{Type: models.CallSignalEnded})
		return nil
	}
	if errors.Is(err, apperrors.ErrCallTokenInvalid) {
		css.hub.ClosePeer(claims.RoomID, claims.PeerID, models.CallSignal{Type: models.CallSignalError, Reason: "you are no longer in this room"})
		return nil
	}
	if err != nil {
		return err
	}
	if signal.Type == models.CallSignalLeave {
		css.hub.ClosePeer(room.ID, self.PeerID, models.CallSignal{Type: models.CallSignalLeave})
		return css.leave(ctx, claims)
	}
	if self.AdmissionStatus != models.CallAdmissionAdmitted {
		css.reject(room.ID, self, "wait for the host to admit you")
		return nil
	}

	switch signal.Type {
	case models.CallSignalOffer, models.CallSignalAnswer, models.CallSignalICE:
		target := findCallParticipant(participants, func(candidate *models.CallParticipant) bool {
			return candidate.PeerID == signal.To
		})
		if target == nil || target.ID == self.ID || !inRoom(target, now) {
			css.reject(room.ID, self, "that peer is not in the room")
			return nil
		}
		css.hub.Send(room.ID, target.PeerID, models.CallSignal{
			Type:      signal.Type,
			From:      self.PeerID,
			SDP:       signal.SDP,
			Candidate: signal.Candidate,
		})
	case models.CallSignalMedia:
		if signal.Mode != models.CallMediaAudio && signal.Mode != models.CallMediaVideo {
			css.reject(room.ID, self, "mode must be audio or video")
			return nil
		}
		self.MediaMode = signal.Mode
		if err := css.record(ctx, self, models.CallEventMediaChanged, string(signal.Mode)); err != nil {
			return err
		}
		css.hub.Broadcast(room.ID, inRoomPeerIDs(participants, self.ID, now), models.CallSignal{Type: models.CallSignalMedia, From: self.PeerID, Mode: signal.Mode})
	case models.CallSignalState:
		if !validConnectionState(signal.State) {
			css.reject(room.ID, self, "state must be connected, relay or failed")
			return nil
		}
		self.ConnectionState = signal.State
		if err := css.record(ctx, self, models.CallEventConnectionState, string(signal.State)); err != nil {
			return err
		}
	case models.CallSignalHost:
		if self.Role != models.CallRoleHost {
			css.reject(room.ID, self, "only the host can do that")
			return nil
		}
		return css.hostAction(ctx, room, participants, self, signal, now)
	case models.CallSignalChat:
		return css.chat(room.ID, participants, self, signal, now)
	case models.CallSignalChatReaction:
		css.chatReaction(room.ID, participants, self, signal, now)
	case models.CallSignalChatDelete, models.CallSignalChatSettings:
		if !moderatesChat(self) {
			css.reject(room.ID, self, "only the host can do that")
			return nil
		}
		css.moderateChat(room.ID, participants, self, signal, now)
	default:
		css.reject(room.ID, self, "unknown message type")
	}

	return nil
}

func (css *CallSessionService) hostAction(
	ctx context.Context,
	room *models.CallRoom,
	participants []*models.CallParticipant,
	host *models.CallParticipant,
	signal models.CallSignal,
	now time.Time,
) error {
	if signal.Action == models.CallHostEnd {
		return css.endByHost(ctx, room, host, now)
	}

	target := findCallParticipant(participants, func(candidate *models.CallParticipant) bool {
		return candidate.PeerID == signal.PeerID
	})
	if target == nil || target.ID == host.ID {
		css.reject(room.ID, host, "that peer is not in the room")
		return nil
	}

	switch signal.Action {
	case models.CallHostAdmit:
		if target.AdmissionStatus != models.CallAdmissionWaiting {
			css.reject(room.ID, host, "that peer is not waiting")
			return nil
		}
		return css.admit(ctx, room, target, now)
	case models.CallHostDeny:
		if target.AdmissionStatus != models.CallAdmissionWaiting {
			css.reject(room.ID, host, "that peer is not waiting")
			return nil
		}
		target.AdmissionStatus = models.CallAdmissionDenied
		target.LeftAt = &now
		if err := css.record(ctx, target, models.CallEventDenied, ""); err != nil {
			return err
		}
		css.hub.ClosePeer(room.ID, target.PeerID, models.CallSignal{Type: models.CallSignalHost, Action: models.CallHostDeny})
	case models.CallHostRemove:
		if target.AdmissionStatus != models.CallAdmissionAdmitted {
			css.reject(room.ID, host, "that peer is not in the room")
			return nil
		}
		target.AdmissionStatus = models.CallAdmissionRemoved
		target.LeftAt = &now
		if err := css.record(ctx, target, models.CallEventRemoved, ""); err != nil {
			return err
		}
		css.hub.ClosePeer(room.ID, target.PeerID, models.CallSignal{Type: models.CallSignalHost, Action: models.CallHostRemove})
		css.hub.Broadcast(room.ID, inRoomPeerIDs(participants, target.ID, now), models.CallSignal{Type: models.CallSignalPeerLeft, PeerID: target.PeerID})
	case models.CallHostMute, models.CallHostUnmute:
		if target.AdmissionStatus != models.CallAdmissionAdmitted {
			css.reject(room.ID, host, "that peer is not in the room")
			return nil
		}
		target.MutedByHost = signal.Action == models.CallHostMute
		eventType := models.CallEventUnmuted
		if target.MutedByHost {
			eventType = models.CallEventMuted
		}
		if err := css.record(ctx, target, eventType, ""); err != nil {
			return err
		}
		css.hub.Broadcast(room.ID, inRoomPeerIDs(participants, 0, now), models.CallSignal{Type: models.CallSignalHost, Action: signal.Action, PeerID: target.PeerID})
	default:
		css.reject(room.ID, host, "action must be admit, deny, remove, mute, unmute or end")
	}

	return nil
}

// admit lets a waiting peer in while the room has space, checked under the
// room lock.
func (css *CallSessionService) admit(ctx context.Context, room *models.CallRoom, target *models.CallParticipant, now time.Time) error {
	var participants []*models.CallParticipant
	err := css.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		locked, err := css.rooms.GetByIDForUpdate(txCtx, room.ID)
		if err != nil {
			return err
		}
		participants, err = css.participants.ListByRoom(txCtx, room.ID)
		if err != nil {
			return fmt.Errorf("list call participants: %w", err)
		}
		if !codeRoomMemberFits(locked, participants, target.ID, now) {
			return apperrors.ErrCallRoomFull
		}

		target.AdmissionStatus = models.CallAdmissionAdmitted
		if _, err := css.participants.Update(txCtx, target); err != nil {
			return fmt.Errorf("admit call participant: %w", err)
		}
		if err := css.events.Create(txCtx, callEvent(room.ID, target.ID, models.CallEventAdmitted, "")); err != nil {
			return fmt.Errorf("create call admitted event: %w", err)
		}
		for i, participant := range participants {
			if participant.ID == target.ID {
				participants[i] = target
			}
		}

		return css.start(txCtx, locked, participants, target, now)
	})
	if errors.Is(err, apperrors.ErrCallRoomFull) {
		host := findCallParticipant(participants, func(candidate *models.CallParticipant) bool {
			return candidate.Role == models.CallRoleHost
		})
		if host != nil {
			css.reject(room.ID, host, "this room is full")
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("admit call participant: %w", err)
	}

	css.welcome(room.ID, participants, target, now)
	css.logger.InfoContext(ctx, "call participant admitted", "call_room_id", room.ID, "call_participant_id", target.ID)

	return nil
}

func (css *CallSessionService) endByHost(ctx context.Context, room *models.CallRoom, host *models.CallParticipant, now time.Time) error {
	var hostUserID uint64
	if host.UserID != nil {
		hostUserID = *host.UserID
	}

	err := css.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		locked, err := css.rooms.GetByIDForUpdate(txCtx, room.ID)
		if err != nil {
			return err
		}

		return endCallRoom(txCtx, css.rooms, css.events, css.outbox, locked, models.CallEndReasonHostEnded, hostUserID, now)
	})
	if err != nil && !errors.Is(err, apperrors.ErrCallRoomNotFound) {
		return fmt.Errorf("end call room by host: %w", err)
	}

	css.hub.CloseRoom(room.ID, models.CallSignal{Type: models.CallSignalEnded, Reason: string(models.CallEndReasonHostEnded)})
	css.forgetChat(room.ID)
	css.logger.InfoContext(ctx, "call room ended by host", "call_room_id", room.ID)

	return nil
}

// start moves an open room to active once a second peer is in, inside the
// caller's transaction.
func (css *CallSessionService) start(
	txCtx context.Context,
	room *models.CallRoom,
	participants []*models.CallParticipant,
	self *models.CallParticipant,
	now time.Time,
) error {
	if room.Status != models.CallRoomStatusOpen || len(inRoomPeerIDs(participants, self.ID, now)) == 0 {
		return nil
	}

	started, err := css.rooms.MarkActive(txCtx, room.ID, now)
	if err != nil {
		return err
	}
	if !started {
		return nil
	}

	active := *room
	active.Status = models.CallRoomStatusActive
	active.StartedAt = &now
	var actingUserID uint64
	if self.UserID != nil {
		actingUserID = *self.UserID
	}

	return css.outbox.Create(txCtx, callRoomEvent(models.OutboxEventCallStarted, &active, actingUserID))
}

// welcome tells an admitted peer who is in the room and tells them it joined.
func (css *CallSessionService) welcome(roomID uint64, participants []*models.CallParticipant, self *models.CallParticipant, now time.Time) {
	peers := make([]models.CallSignalPeer, 0, len(participants))
	for _, participant := range participants {
		if participant.ID != self.ID && inRoom(participant, now) {
			peers = append(peers, signalPeer(participant))
		}
	}

	css.hub.Send(roomID, self.PeerID, models.CallSignal{Type: models.CallSignalWelcome, PeerID: self.PeerID, Peer: peerOf(self), Peers: peers, ChatOff: css.isChatOff(roomID)})
	css.hub.Broadcast(roomID, inRoomPeerIDs(participants, self.ID, now), models.CallSignal{Type: models.CallSignalPeerJoined, Peer: peerOf(self)})
}

func (css *CallSessionService) leave(ctx context.Context, claims models.CallTokenClaims) error {
	now := css.now()
	self, err := css.participants.GetByID(ctx, claims.ParticipantID)
	if err != nil {
		return fmt.Errorf("get leaving call participant: %w", err)
	}
	if self.LeftAt != nil {
		return nil
	}

	self.LeftAt = &now
	if err := css.record(ctx, self, models.CallEventLeft, ""); err != nil {
		return err
	}

	participants, err := css.participants.ListByRoom(ctx, claims.RoomID)
	if err != nil {
		return fmt.Errorf("list call participants: %w", err)
	}
	peerLeft := models.CallSignal{Type: models.CallSignalPeerLeft, PeerID: self.PeerID}
	if self.AdmissionStatus == models.CallAdmissionAdmitted {
		css.hub.Broadcast(claims.RoomID, inRoomPeerIDs(participants, self.ID, now), peerLeft)
	} else {
		css.notifyHost(claims.RoomID, participants, peerLeft, now)
	}

	css.logger.InfoContext(ctx, "call socket left", "call_room_id", claims.RoomID, "call_participant_id", self.ID)

	return nil
}

// load reads the claims' room and participant. An ended room is
// ErrCallRoomNotFound; a participant that does not match the claims, or was
// denied or removed, is ErrCallTokenInvalid.
func (css *CallSessionService) load(
	ctx context.Context,
	claims models.CallTokenClaims,
) (*models.CallRoom, []*models.CallParticipant, *models.CallParticipant, error) {
	room, err := css.rooms.GetByID(ctx, claims.RoomID)
	if err != nil {
		if errors.Is(err, apperrors.ErrCallRoomNotFound) {
			return nil, nil, nil, err
		}
		return nil, nil, nil, fmt.Errorf("get call room: %w", err)
	}
	if room.Status == models.CallRoomStatusEnded {
		css.forgetChat(room.ID)
		return nil, nil, nil, apperrors.ErrCallRoomNotFound
	}

	participants, err := css.participants.ListByRoom(ctx, room.ID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("list call participants: %w", err)
	}
	self := findCallParticipant(participants, func(candidate *models.CallParticipant) bool {
		return candidate.ID == claims.ParticipantID
	})
	if self == nil || self.PeerID != claims.PeerID || !mayConnect(self) {
		return nil, nil, nil, apperrors.ErrCallTokenInvalid
	}

	return room, participants, self, nil
}

// record saves a participant change with its history row.
func (css *CallSessionService) record(ctx context.Context, participant *models.CallParticipant, eventType models.CallEventType, detail string) error {
	return css.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := css.participants.Update(txCtx, participant); err != nil {
			return fmt.Errorf("update call participant: %w", err)
		}
		if err := css.events.Create(txCtx, callEvent(participant.CallRoomID, participant.ID, eventType, detail)); err != nil {
			return fmt.Errorf("create call %s event: %w", eventType, err)
		}

		return nil
	})
}

func (css *CallSessionService) notifyHost(roomID uint64, participants []*models.CallParticipant, signal models.CallSignal, now time.Time) {
	host := findCallParticipant(participants, func(candidate *models.CallParticipant) bool {
		return candidate.Role == models.CallRoleHost && connected(candidate, now)
	})
	if host != nil {
		css.hub.Send(roomID, host.PeerID, signal)
	}
}

func (css *CallSessionService) reject(roomID uint64, participant *models.CallParticipant, reason string) {
	css.hub.Send(roomID, participant.PeerID, models.CallSignal{Type: models.CallSignalError, Reason: reason})
}

func mayConnect(participant *models.CallParticipant) bool {
	return participant.AdmissionStatus == models.CallAdmissionAdmitted || participant.AdmissionStatus == models.CallAdmissionWaiting
}

func peerOf(participant *models.CallParticipant) *models.CallSignalPeer {
	peer := signalPeer(participant)

	return &peer
}
