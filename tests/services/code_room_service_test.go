package services_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/utils"
)

func boolPointer(value bool) *bool {
	return &value
}

func TestCodeRoomServiceCreate(t *testing.T) {
	tests := []struct {
		name             string
		setup            func(f *callFixture)
		maxParticipants  int
		requiresApproval *bool
		scheduledIn      time.Duration
		wantErr          error
		wantLobby        bool
	}{
		{name: "verified doctor", wantLobby: true},
		{name: "without a lobby", requiresApproval: boolPointer(false)},
		{name: "scheduled", scheduledIn: 2 * time.Hour, wantLobby: true},
		{name: "no doctor profile", setup: func(f *callFixture) { f.doctors.byUserID = nil }, wantErr: apperrors.ErrCallHostNotAllowed},
		{name: "doctor not verified", setup: func(f *callFixture) {
			f.doctors.byUserID.VerificationStatus = models.VerificationStatusPending
		}, wantErr: apperrors.ErrCallHostNotAllowed},
		{name: "too many participants", maxParticipants: constant.MaxCodeRoomParticipants + 1, wantErr: apperrors.ErrInvalidCallInput},
		{name: "scheduled in the past", scheduledIn: -time.Minute, wantErr: apperrors.ErrInvalidCallInput},
		{name: "scheduled too far ahead", scheduledIn: constant.MaxCodeRoomScheduleAhead + time.Hour, wantErr: apperrors.ErrInvalidCallInput},
		{name: "too many live rooms", setup: func(f *callFixture) {
			for i := range constant.MaxLiveCodeRoomsPerHost {
				f.rooms.rooms = append(f.rooms.rooms, &models.CallRoom{ID: uint64(i + 1), Kind: models.CallRoomKindCode, HostUserID: callDoctorUserID, Status: models.CallRoomStatusOpen, CreatedAt: f.now})
			}
		}, wantErr: apperrors.ErrCallRoomLimitReached},
		{name: "too many rooms today", setup: func(f *callFixture) {
			for i := range constant.MaxCodeRoomsPerDay {
				f.rooms.rooms = append(f.rooms.rooms, &models.CallRoom{ID: uint64(i + 1), Kind: models.CallRoomKindCode, HostUserID: callDoctorUserID, Status: models.CallRoomStatusEnded, CreatedAt: f.now.Add(-time.Hour)})
			}
		}, wantErr: apperrors.ErrCallRoomLimitReached},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newCallFixture(t)
			if tt.setup != nil {
				tt.setup(f)
			}
			var scheduledAt *time.Time
			if tt.scheduledIn != 0 {
				at := f.now.Add(tt.scheduledIn)
				scheduledAt = &at
			}

			room, code, err := f.codeRoomService(t).Create(context.Background(), callDoctorUserID, " Family briefing ", tt.maxParticipants, tt.requiresApproval, scheduledAt)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Create error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}

			if len(code) != constant.CodeRoomCodeDigits+1 {
				t.Fatalf("code = %q, want %d digits", code, constant.CodeRoomCodeDigits+1)
			}
			check, err := utils.DammCheckDigit(code[:constant.CodeRoomCodeDigits])
			if err != nil || check != code[constant.CodeRoomCodeDigits] {
				t.Fatalf("code %q has check digit %c, want %c", code, code[constant.CodeRoomCodeDigits], check)
			}
			if room.JoinCodeHash != utils.HashToken(code) {
				t.Fatalf("stored code hash = %q, want the hash of the code", room.JoinCodeHash)
			}
			if room.Title != "Family briefing" || room.MaxParticipants != constant.MaxCodeRoomParticipants || room.RequiresApproval != tt.wantLobby {
				t.Fatalf("room = %q/%d/lobby %v, want trimmed title, %d people, lobby %v", room.Title, room.MaxParticipants, room.RequiresApproval, constant.MaxCodeRoomParticipants, tt.wantLobby)
			}
			wantExpiry := f.now.Add(constant.CodeRoomTTL)
			if scheduledAt != nil {
				wantExpiry = scheduledAt.Add(constant.CodeRoomScheduledTTL)
			}
			if room.ExpiresAt == nil || !room.ExpiresAt.Equal(wantExpiry) {
				t.Fatalf("room expires at %v, want %s", room.ExpiresAt, wantExpiry)
			}
		})
	}
}

func TestCodeRoomServiceJoin(t *testing.T) {
	const clientIP = "203.0.113.7"
	ctx := context.Background()

	newRoom := func(t *testing.T, f *callFixture, maxParticipants int, lobby bool, scheduledAt *time.Time) string {
		t.Helper()
		_, code, err := f.codeRoomService(t).Create(ctx, callDoctorUserID, "", maxParticipants, boolPointer(lobby), scheduledAt)
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		return code
	}
	typo := func(code string) string {
		wrong := byte('0' + (code[constant.CodeRoomCodeDigits]-'0'+1)%10)
		return code[:constant.CodeRoomCodeDigits] + string(wrong)
	}

	t.Run("lobby keeps the joiner waiting", func(t *testing.T) {
		f := newCallFixture(t)
		code := newRoom(t, f, 0, true, nil)

		join, err := f.codeRoomService(t).Join(ctx, clientIP, code[:3]+" "+code[3:6]+"-"+code[6:], "Ravi", "")
		if err != nil {
			t.Fatalf("Join returned error: %v", err)
		}
		if join.Participant.AdmissionStatus != models.CallAdmissionWaiting || join.Participant.Role != models.CallRoleMember {
			t.Fatalf("participant = %s/%s, want a waiting member", join.Participant.Role, join.Participant.AdmissionStatus)
		}
	})

	t.Run("without a lobby the room fills up", func(t *testing.T) {
		f := newCallFixture(t)
		code := newRoom(t, f, constant.MinCodeRoomParticipants, false, nil)

		join, err := f.codeRoomService(t).Join(ctx, clientIP, code, "Ravi", "")
		if err != nil {
			t.Fatalf("Join returned error: %v", err)
		}
		if join.Participant.AdmissionStatus != models.CallAdmissionAdmitted {
			t.Fatalf("participant is %s, want admitted", join.Participant.AdmissionStatus)
		}
		f.participants.connect(join.Participant.ID, f.now)
		if _, err := f.codeRoomService(t).Join(ctx, clientIP, code, "Meera", ""); !errors.Is(err, apperrors.ErrCallRoomFull) {
			t.Fatalf("Join a full room error = %v, want %v", err, apperrors.ErrCallRoomFull)
		}
	})

	t.Run("a typo is refused without counting", func(t *testing.T) {
		f := newCallFixture(t)
		code := newRoom(t, f, 0, true, nil)

		if _, err := f.codeRoomService(t).Join(ctx, clientIP, typo(code), "Ravi", ""); !errors.Is(err, apperrors.ErrInvalidCallInput) {
			t.Fatalf("Join with a typo error = %v, want %v", err, apperrors.ErrInvalidCallInput)
		}
		if _, err := f.codeRoomService(t).Join(ctx, clientIP, "12345", "Ravi", ""); !errors.Is(err, apperrors.ErrInvalidCallInput) {
			t.Fatalf("Join with a short code error = %v, want %v", err, apperrors.ErrInvalidCallInput)
		}
		if _, err := f.codeRoomService(t).Join(ctx, clientIP, code, " ", ""); !errors.Is(err, apperrors.ErrInvalidCallInput) {
			t.Fatalf("Join without a name error = %v, want %v", err, apperrors.ErrInvalidCallInput)
		}
		if len(f.attempts.attempts) != 0 {
			t.Fatalf("attempts = %d, want none for client mistakes", len(f.attempts.attempts))
		}
	})

	t.Run("wrong codes are capped per IP", func(t *testing.T) {
		f := newCallFixture(t)
		code := newRoom(t, f, 0, true, nil)
		unknown := "000000"
		check, _ := utils.DammCheckDigit(unknown)
		unknown += string(check)
		if unknown == code {
			t.Skip("drew the unknown code")
		}

		for range constant.MaxFailedCallJoinAttempts {
			if _, err := f.codeRoomService(t).Join(ctx, clientIP, unknown, "Ravi", ""); !errors.Is(err, apperrors.ErrCallJoinInvalid) {
				t.Fatalf("Join with an unknown code error = %v, want %v", err, apperrors.ErrCallJoinInvalid)
			}
		}
		if _, err := f.codeRoomService(t).Join(ctx, clientIP, code, "Ravi", ""); !errors.Is(err, apperrors.ErrCallJoinAttemptsExceeded) {
			t.Fatalf("Join after %d wrong codes error = %v, want %v", constant.MaxFailedCallJoinAttempts, err, apperrors.ErrCallJoinAttemptsExceeded)
		}
		f.now = f.now.Add(constant.CallJoinAttemptWindow)
		if _, err := f.codeRoomService(t).Join(ctx, clientIP, code, "Ravi", ""); err != nil {
			t.Fatalf("Join after the window returned error: %v", err)
		}
	})

	t.Run("a scheduled room opens shortly before", func(t *testing.T) {
		f := newCallFixture(t)
		scheduledAt := f.now.Add(time.Hour)
		code := newRoom(t, f, 0, true, &scheduledAt)

		if _, err := f.codeRoomService(t).Join(ctx, clientIP, code, "Ravi", ""); !errors.Is(err, apperrors.ErrCallNotOpen) {
			t.Fatalf("Join an hour early error = %v, want %v", err, apperrors.ErrCallNotOpen)
		}
		f.now = scheduledAt.Add(-constant.CallJoinLeadTime)
		if _, err := f.codeRoomService(t).Join(ctx, clientIP, code, "Ravi", ""); err != nil {
			t.Fatalf("Join 10 minutes early returned error: %v", err)
		}
	})

	t.Run("an expired room is an invalid code", func(t *testing.T) {
		f := newCallFixture(t)
		code := newRoom(t, f, 0, true, nil)
		f.now = f.now.Add(constant.CodeRoomTTL)

		if _, err := f.codeRoomService(t).Join(ctx, clientIP, code, "Ravi", ""); !errors.Is(err, apperrors.ErrCallJoinInvalid) {
			t.Fatalf("Join an expired room error = %v, want %v", err, apperrors.ErrCallJoinInvalid)
		}
	})
}

func TestCodeRoomServiceHost(t *testing.T) {
	f := newCallFixture(t)
	service := f.codeRoomService(t)
	ctx := context.Background()

	room, _, err := service.Create(ctx, callDoctorUserID, "", 0, nil, nil)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, err := service.JoinAsHost(ctx, 99, room.ID, ""); !errors.Is(err, apperrors.ErrCallRoomNotFound) {
		t.Fatalf("JoinAsHost by another doctor error = %v, want %v", err, apperrors.ErrCallRoomNotFound)
	}

	join, err := service.JoinAsHost(ctx, callDoctorUserID, room.ID, models.CallMediaVideo)
	if err != nil {
		t.Fatalf("JoinAsHost returned error: %v", err)
	}
	again, err := service.JoinAsHost(ctx, callDoctorUserID, room.ID, "")
	if err != nil {
		t.Fatalf("JoinAsHost again returned error: %v", err)
	}
	if join.Participant.Role != models.CallRoleHost || again.Participant.ID != join.Participant.ID {
		t.Fatalf("host participants = %s %d/%d, want one host row", join.Participant.Role, join.Participant.ID, again.Participant.ID)
	}

	rooms, err := service.List(ctx, callDoctorUserID)
	if err != nil || len(rooms) != 1 {
		t.Fatalf("List = %d rooms, %v, want 1", len(rooms), err)
	}
	state, err := service.ListParticipants(ctx, callDoctorUserID, room.ID)
	if err != nil || len(state.Participants) != 1 {
		t.Fatalf("ListParticipants = %v, %v, want the host", state, err)
	}

	if err := service.End(ctx, 99, room.ID); !errors.Is(err, apperrors.ErrCallRoomNotFound) {
		t.Fatalf("End by another doctor error = %v, want %v", err, apperrors.ErrCallRoomNotFound)
	}
	if err := service.End(ctx, callDoctorUserID, room.ID); err != nil {
		t.Fatalf("End returned error: %v", err)
	}
	if ended := f.rooms.rooms[0]; ended.Status != models.CallRoomStatusEnded || ended.EndReason != models.CallEndReasonHostEnded {
		t.Fatalf("room = %s/%s, want ended/host_ended", ended.Status, ended.EndReason)
	}
	if _, err := service.JoinAsHost(ctx, callDoctorUserID, room.ID, ""); !errors.Is(err, apperrors.ErrCallRoomNotFound) {
		t.Fatalf("JoinAsHost an ended room error = %v, want %v", err, apperrors.ErrCallRoomNotFound)
	}
}
