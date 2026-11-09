package controller

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/dtos/call"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
	"context"
)

type CallController struct {
	callService     *services.CallService
	codeRoomService *services.CodeRoomService
	sessionService  *services.CallSessionService
}

func NewCallController(
	callService *services.CallService,
	codeRoomService *services.CodeRoomService,
	sessionService *services.CallSessionService,
) (*CallController, error) {
	if callService == nil {
		return nil, apperrors.ErrNilCallService
	}
	if codeRoomService == nil {
		return nil, apperrors.ErrNilCodeRoomService
	}
	if sessionService == nil {
		return nil, apperrors.ErrNilCallSessionService
	}
	return &CallController{
		callService:     callService,
		codeRoomService: codeRoomService,
		sessionService:  sessionService,
	}, nil
}

// CallSocket is one signaling WebSocket, written by the handler. Send must not
// block.
type CallSocket interface {
	Send(message call.CallSignalMessage) error
	Close()
}

// CallSession is an authorized signaling connection, kept by the handler for
// the life of the socket.
type CallSession struct {
	claims models.CallTokenClaims
	peer   *callSocketPeer
}

func (cc *CallController) OpenAppointmentCall(ctx context.Context, request *call.OpenCallRequest) (*call.CallJoinResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	join, err := cc.callService.Open(ctx, userID, role, request.ID, models.CallMediaMode(request.MediaMode))
	if err != nil {
		return nil, err
	}

	response := toCallJoinResponse(join)

	return &response, nil
}

func (cc *CallController) GetAppointmentCall(ctx context.Context, request *call.GetCallRequest) (*call.GetCallResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	state, err := cc.callService.Get(ctx, userID, role, request.ID)
	if err != nil {
		return nil, err
	}

	return &call.GetCallResponse{
		RoomID:       state.Room.ID,
		Status:       string(state.Room.Status),
		OpenedAt:     state.Room.OpenedAt.UTC(),
		StartedAt:    state.Room.StartedAt,
		Participants: toCallParticipantResponses(state),
	}, nil
}

func (cc *CallController) EndAppointmentCall(ctx context.Context, request *call.EndCallRequest) error {
	userID, err := actingUserID(ctx)
	if err != nil {
		return err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return err
	}

	return cc.callService.End(ctx, userID, role, request.ID)
}

func (cc *CallController) CreateGuestLink(ctx context.Context, request *call.CreateGuestLinkRequest) (*call.GuestLinkResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	createdLink, token, err := cc.callService.CreateGuestLink(ctx, userID, role, request.ID, request.DisplayName, request.MaxUses)
	if err != nil {
		return nil, err
	}

	return &call.GuestLinkResponse{
		ID:          createdLink.ID,
		Token:       token,
		DisplayName: createdLink.DisplayName,
		MaxUses:     createdLink.MaxUses,
		ExpiresAt:   createdLink.ExpiresAt.UTC(),
	}, nil
}

func (cc *CallController) RevokeGuestLink(ctx context.Context, request *call.RevokeGuestLinkRequest) error {
	userID, err := actingUserID(ctx)
	if err != nil {
		return err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return err
	}

	return cc.callService.RevokeGuestLink(ctx, userID, role, request.ID, request.LinkID)
}

func (cc *CallController) JoinGuestCall(ctx context.Context, request *call.JoinGuestCallRequest, clientIP string) (*call.CallJoinResponse, error) {
	join, err := cc.callService.JoinAsGuest(ctx, clientIP, request.Token, request.DisplayName, models.CallMediaMode(request.MediaMode))
	if err != nil {
		return nil, err
	}

	response := toCallJoinResponse(join)

	return &response, nil
}

func (cc *CallController) CreateRoom(ctx context.Context, request *call.CreateCallRoomRequest) (*call.CallRoomResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}

	createdRoom, code, err := cc.codeRoomService.Create(ctx, userID, request.Title, request.MaxParticipants, request.RequiresApproval, request.ScheduledAt)
	if err != nil {
		return nil, err
	}

	response := toCallRoomResponse(createdRoom, code)

	return &response, nil
}

func (cc *CallController) ListRooms(ctx context.Context) (*call.ListCallRoomsResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}

	rooms, err := cc.codeRoomService.List(ctx, userID)
	if err != nil {
		return nil, err
	}

	response := call.ListCallRoomsResponse{
		Rooms: make([]call.CallRoomResponse, 0, len(rooms)),
	}
	for _, room := range rooms {
		response.Rooms = append(response.Rooms, toCallRoomResponse(room, ""))
	}

	return &response, nil
}

func (cc *CallController) EndRoom(ctx context.Context, request *call.EndCallRoomRequest) error {
	userID, err := actingUserID(ctx)
	if err != nil {
		return err
	}

	return cc.codeRoomService.End(ctx, userID, request.ID)
}

func (cc *CallController) ListRoomParticipants(ctx context.Context, request *call.ListCallRoomParticipantsRequest) (*call.ListCallRoomParticipantsResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}

	state, err := cc.codeRoomService.ListParticipants(ctx, userID, request.ID)
	if err != nil {
		return nil, err
	}

	return &call.ListCallRoomParticipantsResponse{Participants: toCallParticipantResponses(state)}, nil
}

func (cc *CallController) JoinRoomAsHost(ctx context.Context, request *call.JoinCallRoomAsHostRequest) (*call.CallJoinResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}

	join, err := cc.codeRoomService.JoinAsHost(ctx, userID, request.ID, models.CallMediaMode(request.MediaMode))
	if err != nil {
		return nil, err
	}

	response := toCallJoinResponse(join)

	return &response, nil
}

func (cc *CallController) JoinRoom(ctx context.Context, request *call.JoinCallRoomRequest, clientIP string) (*call.CallJoinResponse, error) {
	join, err := cc.codeRoomService.Join(ctx, clientIP, request.Code, request.DisplayName, models.CallMediaMode(request.MediaMode))
	if err != nil {
		return nil, err
	}

	response := toCallJoinResponse(join)

	return &response, nil
}

// AuthorizeSocket checks a call token before the WebSocket is upgraded.
func (cc *CallController) AuthorizeSocket(ctx context.Context, request *call.CallSocketRequest) (*CallSession, error) {
	claims, err := cc.sessionService.Authorize(ctx, request.Token)
	if err != nil {
		return nil, err
	}

	return &CallSession{claims: claims}, nil
}

func (cc *CallController) ConnectSocket(ctx context.Context, session *CallSession, socket CallSocket) error {
	session.peer = &callSocketPeer{peerID: session.claims.PeerID, socket: socket}

	return cc.sessionService.Connect(ctx, session.claims, session.peer)
}

func (cc *CallController) HandleSignal(ctx context.Context, session *CallSession, message *call.CallSignalMessage) error {
	return cc.sessionService.Handle(ctx, session.claims, toCallSignal(message))
}

// HeartbeatSocket reports whether the socket's room is still live.
func (cc *CallController) HeartbeatSocket(ctx context.Context, session *CallSession) (bool, error) {
	return cc.sessionService.Heartbeat(ctx, session.claims)
}

func (cc *CallController) DisconnectSocket(ctx context.Context, session *CallSession) error {
	if session.peer == nil {
		return nil
	}

	return cc.sessionService.Disconnect(ctx, session.claims, session.peer)
}

// callSocketPeer lets the services write to a handler's socket.
type callSocketPeer struct {
	peerID string
	socket CallSocket
}

func (csp *callSocketPeer) PeerID() string {
	return csp.peerID
}

func (csp *callSocketPeer) Send(signal models.CallSignal) error {
	return csp.socket.Send(toCallSignalMessage(signal))
}

func (csp *callSocketPeer) Close() {
	csp.socket.Close()
}
