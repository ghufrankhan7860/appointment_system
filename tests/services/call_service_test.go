package services_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"appointment_system/internal/apps/appointment/adapters/callhub/memory"
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
	"appointment_system/internal/apps/appointment/utils"
)

const testCallSecret = "0123456789abcdef0123456789abcdef"

const (
	callDoctorUserID  = 11
	callPatientUserID = 7
)

// callFixture holds the in-memory repositories a call test drives. Its clock
// is now, so a test can move time forward.
type callFixture struct {
	now          time.Time
	appointment  *models.AppointmentDetails
	appointments *fakeAppointmentRepo
	doctors      *fakeDoctorRepo
	userPatients *fakeUserPatientRepo
	users        *fakeUserRepo
	rooms        *fakeCallRoomRepo
	participants *fakeCallParticipantRepo
	guestTokens  *fakeCallGuestTokenRepo
	events       *fakeCallEventRepo
	attempts     *fakeCallJoinAttemptRepo
	outbox       *fakeOutboxRepo
	tokens       *utils.CallTokenManager
	hub          *memory.Hub
	ice          services.ICESettings
}

func newCallFixture(t *testing.T) *callFixture {
	t.Helper()

	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	appointment := &models.AppointmentDetails{Appointment: models.Appointment{
		ID:            5,
		PatientID:     3,
		DoctorID:      2,
		StartsAt:      now,
		EndsAt:        now.Add(30 * time.Minute),
		Mode:          models.ConsultationModeVideo,
		Status:        models.AppointmentStatusScheduled,
		PaymentStatus: models.AppointmentPaymentPaid,
	}}
	tokens, err := utils.NewCallTokenManager(testCallSecret, "test-issuer", constant.CallTokenTTL)
	if err != nil {
		t.Fatalf("NewCallTokenManager returned error: %v", err)
	}

	return &callFixture{
		now:          now,
		appointment:  appointment,
		appointments: &fakeAppointmentRepo{byID: appointment},
		doctors:      &fakeDoctorRepo{byUserID: &models.Doctor{ID: 2, UserID: callDoctorUserID, VerificationStatus: models.VerificationStatusVerified}},
		userPatients: &fakeUserPatientRepo{activeResult: patientLink(models.AccessLevelFull)},
		users:        &fakeUserRepo{user: &models.User{ID: callPatientUserID, Name: "Asha Rao"}},
		rooms:        &fakeCallRoomRepo{},
		participants: &fakeCallParticipantRepo{},
		guestTokens:  &fakeCallGuestTokenRepo{},
		events:       &fakeCallEventRepo{},
		attempts:     &fakeCallJoinAttemptRepo{},
		outbox:       &fakeOutboxRepo{},
		tokens:       tokens,
		hub:          memory.New(),
		ice:          services.ICESettings{STUNURLs: []string{"stun:stun.example.test:3478"}},
	}
}

func (f *callFixture) clock() time.Time {
	return f.now
}

func (f *callFixture) callService(t *testing.T) *services.CallService {
	t.Helper()

	service, err := services.NewCallService(f.appointments, f.doctors, f.userPatients, f.users, f.rooms, f.participants, f.guestTokens, f.events, f.attempts, f.outbox, &fakeTxManager{}, f.tokens, f.hub, f.ice, f.clock, discardLogger)
	if err != nil {
		t.Fatalf("NewCallService returned error: %v", err)
	}
	return service
}

func (f *callFixture) codeRoomService(t *testing.T) *services.CodeRoomService {
	t.Helper()

	service, err := services.NewCodeRoomService(f.doctors, f.users, f.rooms, f.participants, f.events, f.attempts, f.outbox, &fakeTxManager{}, f.tokens, f.hub, f.ice, f.clock, discardLogger)
	if err != nil {
		t.Fatalf("NewCodeRoomService returned error: %v", err)
	}
	return service
}

func (f *callFixture) sessionService(t *testing.T) *services.CallSessionService {
	t.Helper()

	service, err := services.NewCallSessionService(f.rooms, f.participants, f.events, f.outbox, &fakeTxManager{}, f.tokens, f.hub, f.clock, discardLogger)
	if err != nil {
		t.Fatalf("NewCallSessionService returned error: %v", err)
	}
	return service
}

func countOutbox(outbox *fakeOutboxRepo, eventType string) int {
	count := 0
	for _, event := range outbox.events {
		if event.EventType == eventType {
			count++
		}
	}
	return count
}

func TestNewCallServiceValidation(t *testing.T) {
	f := newCallFixture(t)

	tests := []struct {
		name    string
		build   func() error
		wantErr error
	}{
		{name: "nil call room repository", build: func() error {
			_, err := services.NewCallService(f.appointments, f.doctors, f.userPatients, f.users, nil, f.participants, f.guestTokens, f.events, f.attempts, f.outbox, &fakeTxManager{}, f.tokens, f.hub, f.ice, f.clock, discardLogger)
			return err
		}, wantErr: apperrors.ErrNilCallRoomRepository},
		{name: "nil call hub", build: func() error {
			_, err := services.NewCallService(f.appointments, f.doctors, f.userPatients, f.users, f.rooms, f.participants, f.guestTokens, f.events, f.attempts, f.outbox, &fakeTxManager{}, f.tokens, nil, f.ice, f.clock, discardLogger)
			return err
		}, wantErr: apperrors.ErrNilCallHub},
		{name: "nil call token issuer", build: func() error {
			_, err := services.NewCodeRoomService(f.doctors, f.users, f.rooms, f.participants, f.events, f.attempts, f.outbox, &fakeTxManager{}, nil, f.hub, f.ice, f.clock, discardLogger)
			return err
		}, wantErr: apperrors.ErrNilCallTokenIssuer},
		{name: "nil join attempt repository", build: func() error {
			_, err := services.NewCodeRoomService(f.doctors, f.users, f.rooms, f.participants, f.events, nil, f.outbox, &fakeTxManager{}, f.tokens, f.hub, f.ice, f.clock, discardLogger)
			return err
		}, wantErr: apperrors.ErrNilCallJoinAttemptRepository},
		{name: "nil session clock", build: func() error {
			_, err := services.NewCallSessionService(f.rooms, f.participants, f.events, f.outbox, &fakeTxManager{}, f.tokens, f.hub, nil, discardLogger)
			return err
		}, wantErr: apperrors.ErrNilClock},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.build(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("constructor error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestCallServiceOpenAccess(t *testing.T) {
	tests := []struct {
		name     string
		userID   uint64
		role     models.Role
		setup    func(f *callFixture)
		wantRole models.CallParticipantRole
		wantErr  error
	}{
		{name: "appointment doctor", userID: callDoctorUserID, role: models.RoleDoctor, wantRole: models.CallRoleDoctor},
		{name: "patient side with full access", userID: callPatientUserID, role: models.RolePatient, wantRole: models.CallRolePatientSide},
		{name: "patient side with appointments_only access", userID: callPatientUserID, role: models.RolePatient, setup: func(f *callFixture) {
			f.userPatients.activeResult = patientLink(models.AccessLevelAppointmentsOnly)
		}, wantRole: models.CallRolePatientSide},
		{name: "read_only access is not found", userID: callPatientUserID, role: models.RolePatient, setup: func(f *callFixture) {
			f.userPatients.activeResult = patientLink(models.AccessLevelReadOnly)
		}, wantErr: apperrors.ErrAppointmentNotFound},
		{name: "no link is not found", userID: callPatientUserID, role: models.RolePatient, setup: func(f *callFixture) {
			f.userPatients.activeErr = apperrors.ErrUserPatientNotFound
		}, wantErr: apperrors.ErrAppointmentNotFound},
		{name: "another doctor is not found", userID: callDoctorUserID, role: models.RoleDoctor, setup: func(f *callFixture) {
			f.doctors.byUserID = &models.Doctor{ID: 99, UserID: callDoctorUserID}
			f.userPatients.activeErr = apperrors.ErrUserPatientNotFound
		}, wantErr: apperrors.ErrAppointmentNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newCallFixture(t)
			if tt.setup != nil {
				tt.setup(f)
			}

			join, err := f.callService(t).Open(context.Background(), tt.userID, tt.role, f.appointment.ID, "")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Open error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			if join.Participant.Role != tt.wantRole || join.Participant.MediaMode != models.CallMediaAudio {
				t.Fatalf("Open participant = %s/%s, want %s/audio", join.Participant.Role, join.Participant.MediaMode, tt.wantRole)
			}
			claims, err := f.tokens.Verify(join.Token)
			if err != nil {
				t.Fatalf("Verify call token returned error: %v", err)
			}
			if claims.RoomID != join.Room.ID || claims.PeerID != join.Participant.PeerID || claims.Role != tt.wantRole {
				t.Fatalf("call token claims = %+v, want room %d peer %s", claims, join.Room.ID, join.Participant.PeerID)
			}
			if len(join.ICEServers) != 1 || join.ICEServers[0].Username != "" {
				t.Fatalf("ICEServers = %+v, want only the STUN server", join.ICEServers)
			}
		})
	}
}

func TestCallServiceOpenWindow(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(f *callFixture)
		wantErr    error
		wantReason string
	}{
		{name: "nine minutes before the start", setup: func(f *callFixture) { f.now = f.appointment.StartsAt.Add(-9 * time.Minute) }},
		{name: "eleven minutes before the start", setup: func(f *callFixture) {
			f.now = f.appointment.StartsAt.Add(-11 * time.Minute)
		}, wantErr: apperrors.ErrCallNotOpen, wantReason: "opens 10 minutes before"},
		{name: "thirty minutes after the end", setup: func(f *callFixture) {
			f.now = f.appointment.EndsAt.Add(30 * time.Minute)
		}, wantErr: apperrors.ErrCallNotOpen, wantReason: "closed 30 minutes after"},
		{name: "unpaid appointment", setup: func(f *callFixture) {
			f.appointment.PaymentStatus = models.AppointmentPaymentUnpaid
		}, wantErr: apperrors.ErrCallNotOpen, wantReason: "payment is unpaid"},
		{name: "free appointment", setup: func(f *callFixture) { f.appointment.PaymentStatus = models.AppointmentPaymentWaived }},
		{name: "cancelled appointment", setup: func(f *callFixture) {
			f.appointment.Status = models.AppointmentStatusCancelled
		}, wantErr: apperrors.ErrCallNotOpen, wantReason: "appointment is cancelled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newCallFixture(t)
			tt.setup(f)

			_, err := f.callService(t).Open(context.Background(), callDoctorUserID, models.RoleDoctor, f.appointment.ID, models.CallMediaVideo)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Open error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantReason != "" && !strings.Contains(apperrors.Reason(err), tt.wantReason) {
				t.Fatalf("Open reason = %q, want it to contain %q", apperrors.Reason(err), tt.wantReason)
			}
		})
	}
}

func TestCallServiceOpenSeats(t *testing.T) {
	f := newCallFixture(t)
	service := f.callService(t)
	ctx := context.Background()

	doctorJoin, err := service.Open(ctx, callDoctorUserID, models.RoleDoctor, f.appointment.ID, "")
	if err != nil {
		t.Fatalf("Open doctor returned error: %v", err)
	}
	f.participants.connect(doctorJoin.Participant.ID, f.now)

	patientJoin, err := service.Open(ctx, callPatientUserID, models.RolePatient, f.appointment.ID, "")
	if err != nil {
		t.Fatalf("Open patient returned error: %v", err)
	}
	if patientJoin.Room.ID != doctorJoin.Room.ID {
		t.Fatalf("patient joined room %d, want the doctor's room %d", patientJoin.Room.ID, doctorJoin.Room.ID)
	}
	f.participants.connect(patientJoin.Participant.ID, f.now)

	if _, err := service.Open(ctx, 8, models.RolePatient, f.appointment.ID, ""); !errors.Is(err, apperrors.ErrCallRoomFull) {
		t.Fatalf("Open second patient-side user error = %v, want %v", err, apperrors.ErrCallRoomFull)
	}

	again, err := service.Open(ctx, callPatientUserID, models.RolePatient, f.appointment.ID, models.CallMediaVideo)
	if err != nil {
		t.Fatalf("Open patient again returned error: %v", err)
	}
	if again.Participant.ID != patientJoin.Participant.ID || again.Participant.MediaMode != models.CallMediaVideo {
		t.Fatalf("rejoin participant = %d/%s, want %d/video", again.Participant.ID, again.Participant.MediaMode, patientJoin.Participant.ID)
	}

	_, token, err := service.CreateGuestLink(ctx, callDoctorUserID, models.RoleDoctor, f.appointment.ID, "Ravi", 0)
	if err != nil {
		t.Fatalf("CreateGuestLink returned error: %v", err)
	}
	if _, err := service.JoinAsGuest(ctx, "203.0.113.5", token, "", ""); !errors.Is(err, apperrors.ErrCallRoomFull) {
		t.Fatalf("JoinAsGuest while the patient side is in error = %v, want %v", err, apperrors.ErrCallRoomFull)
	}

	if len(f.rooms.rooms) != 1 || f.events.count(models.CallEventOpened) != 1 {
		t.Fatalf("rooms = %d, opened events = %d, want one room opened once", len(f.rooms.rooms), f.events.count(models.CallEventOpened))
	}
}

func TestCallServiceGuestLinks(t *testing.T) {
	const clientIP = "203.0.113.9"
	f := newCallFixture(t)
	service := f.callService(t)
	ctx := context.Background()

	if _, _, err := service.CreateGuestLink(ctx, callPatientUserID, models.RolePatient, f.appointment.ID, "", 0); !errors.Is(err, apperrors.ErrAppointmentNotFound) {
		t.Fatalf("CreateGuestLink by the patient side error = %v, want %v", err, apperrors.ErrAppointmentNotFound)
	}
	if _, _, err := service.CreateGuestLink(ctx, callDoctorUserID, models.RoleDoctor, f.appointment.ID, "", constant.MaxGuestLinkUses+1); !errors.Is(err, apperrors.ErrInvalidCallInput) {
		t.Fatalf("CreateGuestLink with too many uses error = %v, want %v", err, apperrors.ErrInvalidCallInput)
	}

	link, token, err := service.CreateGuestLink(ctx, callDoctorUserID, models.RoleDoctor, f.appointment.ID, "", 0)
	if err != nil {
		t.Fatalf("CreateGuestLink returned error: %v", err)
	}
	if link.MaxUses != constant.DefaultGuestLinkUses || !link.ExpiresAt.Equal(f.appointment.EndsAt.Add(constant.CallJoinGrace)) {
		t.Fatalf("guest link = %d uses until %s, want %d until 30 minutes after the end", link.MaxUses, link.ExpiresAt, constant.DefaultGuestLinkUses)
	}
	if f.guestTokens.tokens[0].TokenHash != utils.HashToken(token) || strings.Contains(f.guestTokens.tokens[0].TokenHash, token) {
		t.Fatalf("stored guest link keeps %q, want only the hash of the token", f.guestTokens.tokens[0].TokenHash)
	}

	if _, err := service.JoinAsGuest(ctx, clientIP, token, "", ""); !errors.Is(err, apperrors.ErrInvalidCallInput) {
		t.Fatalf("JoinAsGuest without a name error = %v, want %v", err, apperrors.ErrInvalidCallInput)
	}
	for use := 1; use <= constant.DefaultGuestLinkUses; use++ {
		join, err := service.JoinAsGuest(ctx, clientIP, token, "Ravi", "")
		if err != nil {
			t.Fatalf("JoinAsGuest use %d returned error: %v", use, err)
		}
		if join.Participant.Role != models.CallRoleGuest || join.Participant.UserID != nil || join.Participant.DisplayName != "Ravi" {
			t.Fatalf("guest participant = %+v, want a guest named Ravi without a user", join.Participant)
		}
	}
	if _, err := service.JoinAsGuest(ctx, clientIP, token, "Ravi", ""); !errors.Is(err, apperrors.ErrCallJoinInvalid) {
		t.Fatalf("JoinAsGuest after the last use error = %v, want %v", err, apperrors.ErrCallJoinInvalid)
	}
	if len(f.attempts.attempts) != 1 {
		t.Fatalf("attempts = %d, want the used-up link counted once", len(f.attempts.attempts))
	}

	second, secondToken, err := service.CreateGuestLink(ctx, callDoctorUserID, models.RoleDoctor, f.appointment.ID, "", 1)
	if err != nil {
		t.Fatalf("CreateGuestLink returned error: %v", err)
	}
	if err := service.RevokeGuestLink(ctx, callDoctorUserID, models.RoleDoctor, f.appointment.ID, second.ID); err != nil {
		t.Fatalf("RevokeGuestLink returned error: %v", err)
	}
	if err := service.RevokeGuestLink(ctx, callDoctorUserID, models.RoleDoctor, f.appointment.ID, second.ID); !errors.Is(err, apperrors.ErrCallGuestLinkNotFound) {
		t.Fatalf("RevokeGuestLink twice error = %v, want %v", err, apperrors.ErrCallGuestLinkNotFound)
	}
	for range constant.MaxFailedCallJoinAttempts - 1 {
		if _, err := service.JoinAsGuest(ctx, clientIP, secondToken, "Ravi", ""); !errors.Is(err, apperrors.ErrCallJoinInvalid) {
			t.Fatalf("JoinAsGuest with a revoked link error = %v, want %v", err, apperrors.ErrCallJoinInvalid)
		}
	}
	if _, err := service.JoinAsGuest(ctx, clientIP, "unknown-token", "Ravi", ""); !errors.Is(err, apperrors.ErrCallJoinAttemptsExceeded) {
		t.Fatalf("JoinAsGuest after %d failures error = %v, want %v", constant.MaxFailedCallJoinAttempts, err, apperrors.ErrCallJoinAttemptsExceeded)
	}
	if _, err := service.JoinAsGuest(ctx, "198.51.100.1", "unknown-token", "Ravi", ""); !errors.Is(err, apperrors.ErrCallJoinInvalid) {
		t.Fatalf("JoinAsGuest from another IP error = %v, want %v", err, apperrors.ErrCallJoinInvalid)
	}
}

func TestCallServiceEnd(t *testing.T) {
	f := newCallFixture(t)
	service := f.callService(t)
	ctx := context.Background()

	join, err := service.Open(ctx, callDoctorUserID, models.RoleDoctor, f.appointment.ID, "")
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	peer := &fakeCallPeer{peerID: join.Participant.PeerID}
	f.hub.Join(join.Room.ID, peer)

	if err := service.End(ctx, callPatientUserID, models.RolePatient, f.appointment.ID); err != nil {
		t.Fatalf("End returned error: %v", err)
	}
	if room := f.rooms.rooms[0]; room.Status != models.CallRoomStatusEnded || room.EndReason != models.CallEndReasonHangup {
		t.Fatalf("room = %s/%s, want ended/hangup", room.Status, room.EndReason)
	}
	if !peer.closed || len(peer.received(models.CallSignalEnded)) != 1 {
		t.Fatalf("peer closed = %v with %d ended signals, want closed after one", peer.closed, len(peer.received(models.CallSignalEnded)))
	}
	if countOutbox(f.outbox, models.OutboxEventCallEnded) != 1 || f.events.count(models.CallEventEnded) != 1 {
		t.Fatalf("call.ended events = %d, ended history rows = %d, want one each", countOutbox(f.outbox, models.OutboxEventCallEnded), f.events.count(models.CallEventEnded))
	}
	if err := service.End(ctx, callPatientUserID, models.RolePatient, f.appointment.ID); !errors.Is(err, apperrors.ErrCallRoomNotFound) {
		t.Fatalf("End twice error = %v, want %v", err, apperrors.ErrCallRoomNotFound)
	}

	reopened, err := service.Open(ctx, callDoctorUserID, models.RoleDoctor, f.appointment.ID, "")
	if err != nil {
		t.Fatalf("Open after hangup returned error: %v", err)
	}
	if reopened.Room.ID == join.Room.ID {
		t.Fatalf("Open after hangup reused room %d, want a new room", join.Room.ID)
	}
}

func TestCallServiceExpire(t *testing.T) {
	appointmentID := uint64(5)
	tests := []struct {
		name       string
		setup      func(f *callFixture) *models.CallRoom
		wantReason models.CallEndReason
	}{
		{name: "appointment room nobody answered", setup: func(f *callFixture) *models.CallRoom {
			f.now = f.appointment.EndsAt.Add(constant.CallJoinGrace)
			return &models.CallRoom{Kind: models.CallRoomKindAppointment, AppointmentID: &appointmentID, Status: models.CallRoomStatusOpen}
		}, wantReason: models.CallEndReasonNoAnswer},
		{name: "appointment room past its window", setup: func(f *callFixture) *models.CallRoom {
			f.now = f.appointment.EndsAt.Add(constant.CallJoinGrace)
			started := f.appointment.StartsAt
			return &models.CallRoom{Kind: models.CallRoomKindAppointment, AppointmentID: &appointmentID, Status: models.CallRoomStatusActive, StartedAt: &started}
		}, wantReason: models.CallEndReasonTimeout},
		{name: "appointment completed elsewhere", setup: func(f *callFixture) *models.CallRoom {
			f.appointment.Status = models.AppointmentStatusCompleted
			return &models.CallRoom{Kind: models.CallRoomKindAppointment, AppointmentID: &appointmentID, Status: models.CallRoomStatusActive}
		}, wantReason: models.CallEndReasonAppointmentClosed},
		{name: "appointment room in its window", setup: func(f *callFixture) *models.CallRoom {
			return &models.CallRoom{Kind: models.CallRoomKindAppointment, AppointmentID: &appointmentID, Status: models.CallRoomStatusOpen}
		}},
		{name: "code room past its expiry", setup: func(f *callFixture) *models.CallRoom {
			expiresAt := f.now
			return &models.CallRoom{Kind: models.CallRoomKindCode, JoinCodeHash: "hash", Status: models.CallRoomStatusOpen, OpenedAt: f.now, ExpiresAt: &expiresAt}
		}, wantReason: models.CallEndReasonExpired},
		{name: "code room left by its host", setup: func(f *callFixture) *models.CallRoom {
			left := f.now.Add(-constant.CodeRoomHostGrace)
			f.participants.participants = append(f.participants.participants, &models.CallParticipant{ID: 1, CallRoomID: 1, Role: models.CallRoleHost, AdmissionStatus: models.CallAdmissionAdmitted, JoinedAt: &left, LastSeenAt: &left, LeftAt: &left})
			seen := f.now
			f.participants.participants = append(f.participants.participants, &models.CallParticipant{ID: 2, CallRoomID: 1, Role: models.CallRoleMember, AdmissionStatus: models.CallAdmissionAdmitted, JoinedAt: &seen, LastSeenAt: &seen})
			return &models.CallRoom{Kind: models.CallRoomKindCode, JoinCodeHash: "hash", Status: models.CallRoomStatusActive, OpenedAt: f.now.Add(-time.Hour)}
		}, wantReason: models.CallEndReasonHostEnded},
		{name: "code room nobody has been in", setup: func(f *callFixture) *models.CallRoom {
			return &models.CallRoom{Kind: models.CallRoomKindCode, JoinCodeHash: "hash", Status: models.CallRoomStatusOpen, OpenedAt: f.now.Add(-constant.CodeRoomIdleTimeout)}
		}, wantReason: models.CallEndReasonTimeout},
		{name: "code room in use", setup: func(f *callFixture) *models.CallRoom {
			seen := f.now
			f.participants.participants = append(f.participants.participants, &models.CallParticipant{ID: 1, CallRoomID: 1, Role: models.CallRoleMember, AdmissionStatus: models.CallAdmissionAdmitted, JoinedAt: &seen, LastSeenAt: &seen})
			return &models.CallRoom{Kind: models.CallRoomKindCode, JoinCodeHash: "hash", Status: models.CallRoomStatusOpen, OpenedAt: f.now.Add(-time.Hour)}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newCallFixture(t)
			room := tt.setup(f)
			room.ID = 1
			f.rooms.rooms = append(f.rooms.rooms, room)

			ended, err := f.callService(t).Expire(context.Background(), 10)
			if err != nil {
				t.Fatalf("Expire returned error: %v", err)
			}
			if tt.wantReason == "" {
				if ended != 0 || room.Status == models.CallRoomStatusEnded {
					t.Fatalf("Expire ended %d rooms, want the live room kept", ended)
				}
				return
			}
			if ended != 1 || room.Status != models.CallRoomStatusEnded || room.EndReason != tt.wantReason {
				t.Fatalf("Expire ended %d, room = %s/%s, want 1 ended/%s", ended, room.Status, room.EndReason, tt.wantReason)
			}
			if countOutbox(f.outbox, models.OutboxEventCallEnded) != 1 {
				t.Fatalf("call.ended events = %d, want 1", countOutbox(f.outbox, models.OutboxEventCallEnded))
			}
		})
	}
}

func TestAppointmentServiceClosingEndsCall(t *testing.T) {
	location := clinicLocation(t)
	startsAt := time.Date(2026, 11, 9, 10, 0, 0, 0, location)

	tests := []struct {
		name  string
		now   time.Time
		close func(service *services.AppointmentService) error
	}{
		{name: "complete", now: startsAt.Add(20 * time.Minute), close: func(service *services.AppointmentService) error {
			_, err := service.Complete(context.Background(), callDoctorUserID, models.RoleDoctor, 5)
			return err
		}},
		{name: "no-show", now: startsAt.Add(20 * time.Minute), close: func(service *services.AppointmentService) error {
			_, err := service.MarkNoShow(context.Background(), callDoctorUserID, models.RoleDoctor, 5)
			return err
		}},
		{name: "doctor cancels", now: startsAt.Add(-5 * time.Minute), close: func(service *services.AppointmentService) error {
			_, err := service.Cancel(context.Background(), callDoctorUserID, models.RoleDoctor, 5, "")
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appointmentID := uint64(5)
			doctors := &fakeDoctorRepo{byUserID: &models.Doctor{ID: 2, UserID: callDoctorUserID}}
			appointments := &fakeAppointmentRepo{byID: &models.AppointmentDetails{Appointment: models.Appointment{
				ID: appointmentID, PatientID: 3, DoctorID: 2, StartsAt: startsAt, EndsAt: startsAt.Add(30 * time.Minute),
				Status: models.AppointmentStatusScheduled, PaymentStatus: models.AppointmentPaymentWaived,
			}}}
			outbox := &fakeOutboxRepo{}
			rooms := &fakeCallRoomRepo{rooms: []*models.CallRoom{{ID: 1, Kind: models.CallRoomKindAppointment, AppointmentID: &appointmentID, Status: models.CallRoomStatusActive}}}
			events := &fakeCallEventRepo{}

			availability, err := services.NewAvailabilityService(doctors, &fakeAvailabilityRepo{}, appointments, &fakeTxManager{}, location, func() time.Time { return tt.now }, discardLogger)
			if err != nil {
				t.Fatalf("NewAvailabilityService returned error: %v", err)
			}
			payments, err := services.NewPaymentService(&fakePaymentRepo{}, appointments, outbox, &fakeUserPatientRepo{}, doctors, &fakeGateway{}, &fakeTxManager{}, func() time.Time { return tt.now }, discardLogger)
			if err != nil {
				t.Fatalf("NewPaymentService returned error: %v", err)
			}
			service, err := services.NewAppointmentService(appointments, outbox, &fakeUserPatientRepo{}, doctors, availability, payments, rooms, events, &fakeTxManager{}, discardLogger)
			if err != nil {
				t.Fatalf("NewAppointmentService returned error: %v", err)
			}

			if err := tt.close(service); err != nil {
				t.Fatalf("%s returned error: %v", tt.name, err)
			}
			if room := rooms.rooms[0]; room.Status != models.CallRoomStatusEnded || room.EndReason != models.CallEndReasonAppointmentClosed {
				t.Fatalf("room = %s/%s, want ended/appointment_closed", room.Status, room.EndReason)
			}
			if countOutbox(outbox, models.OutboxEventCallEnded) != 1 || events.count(models.CallEventEnded) != 1 {
				t.Fatalf("call.ended events = %d, ended rows = %d, want one each", countOutbox(outbox, models.OutboxEventCallEnded), events.count(models.CallEventEnded))
			}
		})
	}
}

func TestCallServiceTURNCredentials(t *testing.T) {
	f := newCallFixture(t)
	f.ice.TURNURLs = []string{"turn:turn.example.test:3478"}
	f.ice.TURNSecret = testCallSecret

	join, err := f.callService(t).Open(context.Background(), callDoctorUserID, models.RoleDoctor, f.appointment.ID, "")
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	if len(join.ICEServers) != 2 {
		t.Fatalf("ICEServers = %d, want STUN and TURN", len(join.ICEServers))
	}
	turn := join.ICEServers[1]
	username, credential := utils.TURNCredential(testCallSecret, join.Participant.PeerID, f.now.Add(constant.TURNCredentialTTL))
	if turn.Username != username || turn.Credential != credential {
		t.Fatalf("TURN credentials = %q/%q, want %q/%q", turn.Username, turn.Credential, username, credential)
	}
}
