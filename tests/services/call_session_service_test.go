package services_test

import (
	"context"
	"errors"
	"testing"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
)

// connectCall authorizes a join's token and connects a fake socket for it.
func connectCall(t *testing.T, f *callFixture, sessions *services.CallSessionService, join *models.CallJoin) (models.CallTokenClaims, *fakeCallPeer) {
	t.Helper()

	claims, err := sessions.Authorize(context.Background(), join.Token)
	if err != nil {
		t.Fatalf("Authorize returned error: %v", err)
	}
	peer := &fakeCallPeer{peerID: claims.PeerID}
	if err := sessions.Connect(context.Background(), claims, peer); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	return claims, peer
}

func TestCallSessionServiceAppointmentCall(t *testing.T) {
	f := newCallFixture(t)
	calls := f.callService(t)
	sessions := f.sessionService(t)
	ctx := context.Background()

	doctorJoin, err := calls.Open(ctx, callDoctorUserID, models.RoleDoctor, f.appointment.ID, "")
	if err != nil {
		t.Fatalf("Open doctor returned error: %v", err)
	}
	patientJoin, err := calls.Open(ctx, callPatientUserID, models.RolePatient, f.appointment.ID, "")
	if err != nil {
		t.Fatalf("Open patient returned error: %v", err)
	}

	_, doctorPeer := connectCall(t, f, sessions, doctorJoin)
	if welcome := doctorPeer.received(models.CallSignalWelcome); len(welcome) != 1 || len(welcome[0].Peers) != 0 {
		t.Fatalf("doctor welcome = %+v, want one with nobody in yet", welcome)
	}
	if f.rooms.rooms[0].Status != models.CallRoomStatusOpen {
		t.Fatalf("room is %s with one peer, want open", f.rooms.rooms[0].Status)
	}

	patientClaims, patientPeer := connectCall(t, f, sessions, patientJoin)
	welcome := patientPeer.received(models.CallSignalWelcome)
	if len(welcome) != 1 || len(welcome[0].Peers) != 1 || welcome[0].Peers[0].PeerID != doctorJoin.Participant.PeerID {
		t.Fatalf("patient welcome = %+v, want the doctor listed", welcome)
	}
	if joined := doctorPeer.received(models.CallSignalPeerJoined); len(joined) != 1 || joined[0].Peer.PeerID != patientJoin.Participant.PeerID {
		t.Fatalf("doctor peer-joined = %+v, want the patient", joined)
	}
	if f.rooms.rooms[0].Status != models.CallRoomStatusActive || countOutbox(f.outbox, models.OutboxEventCallStarted) != 1 {
		t.Fatalf("room is %s with %d call.started events, want active with one", f.rooms.rooms[0].Status, countOutbox(f.outbox, models.OutboxEventCallStarted))
	}

	if err := sessions.Handle(ctx, patientClaims, models.CallSignal{Type: models.CallSignalOffer, To: doctorJoin.Participant.PeerID, SDP: "v=0"}); err != nil {
		t.Fatalf("Handle offer returned error: %v", err)
	}
	if offers := doctorPeer.received(models.CallSignalOffer); len(offers) != 1 || offers[0].From != patientJoin.Participant.PeerID || offers[0].SDP != "v=0" {
		t.Fatalf("doctor offers = %+v, want the patient's offer", offers)
	}

	if err := sessions.Handle(ctx, patientClaims, models.CallSignal{Type: models.CallSignalICE, To: "nobody"}); err != nil {
		t.Fatalf("Handle ice returned error: %v", err)
	}
	if err := sessions.Handle(ctx, patientClaims, models.CallSignal{Type: models.CallSignalHost, Action: models.CallHostEnd}); err != nil {
		t.Fatalf("Handle host returned error: %v", err)
	}
	if rejected := patientPeer.received(models.CallSignalError); len(rejected) != 2 {
		t.Fatalf("patient errors = %+v, want the unknown peer and the host action refused", rejected)
	}
	if f.rooms.rooms[0].Status == models.CallRoomStatusEnded {
		t.Fatalf("a patient's host action ended the room")
	}

	if err := sessions.Handle(ctx, patientClaims, models.CallSignal{Type: models.CallSignalMedia, Mode: models.CallMediaVideo}); err != nil {
		t.Fatalf("Handle media returned error: %v", err)
	}
	if media := doctorPeer.received(models.CallSignalMedia); len(media) != 1 || media[0].Mode != models.CallMediaVideo {
		t.Fatalf("doctor media = %+v, want the patient on video", media)
	}
	if err := sessions.Handle(ctx, patientClaims, models.CallSignal{Type: models.CallSignalState, State: models.CallConnectionRelay}); err != nil {
		t.Fatalf("Handle state returned error: %v", err)
	}
	stored, _ := f.participants.GetByID(ctx, patientJoin.Participant.ID)
	if stored.MediaMode != models.CallMediaVideo || stored.ConnectionState != models.CallConnectionRelay {
		t.Fatalf("patient participant = %s/%s, want video/relay saved", stored.MediaMode, stored.ConnectionState)
	}

	if err := sessions.Disconnect(ctx, patientClaims, patientPeer); err != nil {
		t.Fatalf("Disconnect returned error: %v", err)
	}
	if left := doctorPeer.received(models.CallSignalPeerLeft); len(left) != 1 || left[0].PeerID != patientJoin.Participant.PeerID {
		t.Fatalf("doctor peer-left = %+v, want the patient", left)
	}
	stored, _ = f.participants.GetByID(ctx, patientJoin.Participant.ID)
	if stored.LeftAt == nil || f.events.count(models.CallEventLeft) != 1 {
		t.Fatalf("patient left at %v with %d left events, want it recorded once", stored.LeftAt, f.events.count(models.CallEventLeft))
	}

	if _, err := sessions.Authorize(ctx, "not-a-token"); !errors.Is(err, apperrors.ErrCallTokenInvalid) {
		t.Fatalf("Authorize a bad token error = %v, want %v", err, apperrors.ErrCallTokenInvalid)
	}
}

func TestCallSessionServiceReplacedSocket(t *testing.T) {
	f := newCallFixture(t)
	sessions := f.sessionService(t)
	ctx := context.Background()

	join, err := f.callService(t).Open(ctx, callDoctorUserID, models.RoleDoctor, f.appointment.ID, "")
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	claims, first := connectCall(t, f, sessions, join)
	_, second := connectCall(t, f, sessions, join)

	if !first.closed {
		t.Fatalf("the first socket stayed open after the same participant reconnected")
	}
	if err := sessions.Disconnect(ctx, claims, first); err != nil {
		t.Fatalf("Disconnect returned error: %v", err)
	}
	stored, _ := f.participants.GetByID(ctx, join.Participant.ID)
	if stored.LeftAt != nil || second.closed {
		t.Fatalf("closing the replaced socket marked the participant left")
	}
}

func TestCallSessionServiceCodeRoomHost(t *testing.T) {
	f := newCallFixture(t)
	rooms := f.codeRoomService(t)
	sessions := f.sessionService(t)
	ctx := context.Background()

	room, code, err := rooms.Create(ctx, callDoctorUserID, "", 2, nil, nil)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	hostJoin, err := rooms.JoinAsHost(ctx, callDoctorUserID, room.ID, "")
	if err != nil {
		t.Fatalf("JoinAsHost returned error: %v", err)
	}
	hostClaims, hostPeer := connectCall(t, f, sessions, hostJoin)

	memberJoin, err := rooms.Join(ctx, "203.0.113.7", code, "Ravi", "")
	if err != nil {
		t.Fatalf("Join returned error: %v", err)
	}
	memberClaims, memberPeer := connectCall(t, f, sessions, memberJoin)
	if len(memberPeer.received(models.CallSignalLobby)) != 1 {
		t.Fatalf("waiting member signals = %+v, want a lobby notice", memberPeer.signals)
	}
	if lobby := hostPeer.received(models.CallSignalLobby); len(lobby) != 1 || lobby[0].Peer.DisplayName != "Ravi" {
		t.Fatalf("host lobby = %+v, want Ravi waiting", lobby)
	}

	if err := sessions.Handle(ctx, memberClaims, models.CallSignal{Type: models.CallSignalOffer, To: hostJoin.Participant.PeerID}); err != nil {
		t.Fatalf("Handle offer returned error: %v", err)
	}
	if len(memberPeer.received(models.CallSignalError)) != 1 || len(hostPeer.received(models.CallSignalOffer)) != 0 {
		t.Fatalf("a waiting member's offer was relayed")
	}

	if err := sessions.Handle(ctx, hostClaims, models.CallSignal{Type: models.CallSignalHost, Action: models.CallHostAdmit, PeerID: memberJoin.Participant.PeerID}); err != nil {
		t.Fatalf("Handle admit returned error: %v", err)
	}
	if welcome := memberPeer.received(models.CallSignalWelcome); len(welcome) != 1 || len(welcome[0].Peers) != 1 {
		t.Fatalf("admitted member welcome = %+v, want the host listed", welcome)
	}
	if f.rooms.rooms[0].Status != models.CallRoomStatusActive {
		t.Fatalf("room is %s after the admit, want active", f.rooms.rooms[0].Status)
	}

	extraJoin, err := rooms.Join(ctx, "203.0.113.8", code, "Meera", "")
	if err != nil {
		t.Fatalf("Join returned error: %v", err)
	}
	_, extraPeer := connectCall(t, f, sessions, extraJoin)
	if err := sessions.Handle(ctx, hostClaims, models.CallSignal{Type: models.CallSignalHost, Action: models.CallHostAdmit, PeerID: extraJoin.Participant.PeerID}); err != nil {
		t.Fatalf("Handle admit returned error: %v", err)
	}
	if len(hostPeer.received(models.CallSignalError)) != 1 || len(extraPeer.received(models.CallSignalWelcome)) != 0 {
		t.Fatalf("admitting past the room size was not refused")
	}
	if err := sessions.Handle(ctx, hostClaims, models.CallSignal{Type: models.CallSignalHost, Action: models.CallHostDeny, PeerID: extraJoin.Participant.PeerID}); err != nil {
		t.Fatalf("Handle deny returned error: %v", err)
	}
	if !extraPeer.closed {
		t.Fatalf("the denied member's socket stayed open")
	}
	if _, err := sessions.Authorize(ctx, extraJoin.Token); !errors.Is(err, apperrors.ErrCallTokenInvalid) {
		t.Fatalf("Authorize a denied member error = %v, want %v", err, apperrors.ErrCallTokenInvalid)
	}

	if err := sessions.Handle(ctx, hostClaims, models.CallSignal{Type: models.CallSignalHost, Action: models.CallHostMute, PeerID: memberJoin.Participant.PeerID}); err != nil {
		t.Fatalf("Handle mute returned error: %v", err)
	}
	stored, _ := f.participants.GetByID(ctx, memberJoin.Participant.ID)
	if !stored.MutedByHost || len(memberPeer.received(models.CallSignalHost)) != 1 {
		t.Fatalf("member muted = %v, want muted and told so", stored.MutedByHost)
	}

	if err := sessions.Handle(ctx, hostClaims, models.CallSignal{Type: models.CallSignalHost, Action: models.CallHostRemove, PeerID: memberJoin.Participant.PeerID}); err != nil {
		t.Fatalf("Handle remove returned error: %v", err)
	}
	if !memberPeer.closed || len(hostPeer.received(models.CallSignalPeerLeft)) != 1 {
		t.Fatalf("removed member closed = %v, host told %d times", memberPeer.closed, len(hostPeer.received(models.CallSignalPeerLeft)))
	}

	if err := sessions.Handle(ctx, hostClaims, models.CallSignal{Type: models.CallSignalHost, Action: models.CallHostEnd}); err != nil {
		t.Fatalf("Handle end returned error: %v", err)
	}
	if ended := f.rooms.rooms[0]; ended.Status != models.CallRoomStatusEnded || ended.EndReason != models.CallEndReasonHostEnded || !hostPeer.closed {
		t.Fatalf("room = %s/%s, host closed %v, want ended by the host", ended.Status, ended.EndReason, hostPeer.closed)
	}
	if live, err := sessions.Heartbeat(ctx, hostClaims); err != nil || live {
		t.Fatalf("Heartbeat after the end = %v, %v, want not live", live, err)
	}
}
