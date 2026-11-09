package repositories_test

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/repositories"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"
)

func newTestCallRoomRepository(t *testing.T, db *gorm.DB) *repositories.CallRoomRepository {
	t.Helper()

	rooms, err := repositories.NewCallRoomRepository(db)
	if err != nil {
		t.Fatalf("NewCallRoomRepository returned error: %v", err)
	}
	return rooms
}

func newTestCallEventRepository(t *testing.T, db *gorm.DB) *repositories.CallEventRepository {
	t.Helper()

	events, err := repositories.NewCallEventRepository(db)
	if err != nil {
		t.Fatalf("NewCallEventRepository returned error: %v", err)
	}
	return events
}

func TestCallRepositories(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	fixture := newAppointmentFixture(t, db)
	clientIP := fmt.Sprintf("198.51.100.%d", time.Now().UnixNano()%250+1)

	rooms := newTestCallRoomRepository(t, db)
	events := newTestCallEventRepository(t, db)
	participants, err := repositories.NewCallParticipantRepository(db)
	if err != nil {
		t.Fatalf("NewCallParticipantRepository returned error: %v", err)
	}
	guestTokens, err := repositories.NewCallGuestTokenRepository(db)
	if err != nil {
		t.Fatalf("NewCallGuestTokenRepository returned error: %v", err)
	}
	attempts, err := repositories.NewCallJoinAttemptRepository(db)
	if err != nil {
		t.Fatalf("NewCallJoinAttemptRepository returned error: %v", err)
	}
	txManager, err := repositories.NewGORMTransactionManager(db)
	if err != nil {
		t.Fatalf("NewGORMTransactionManager returned error: %v", err)
	}

	appointment, err := fixture.book(time.Now().UTC().Add(24*time.Hour).Truncate(time.Minute), 30)
	if err != nil {
		t.Fatalf("book returned error: %v", err)
	}
	hostID := fixture.booker.ID
	t.Cleanup(func() {
		db.Exec("DELETE FROM call_join_attempts WHERE client_ip = ?", clientIP)
		db.Exec("DELETE FROM call_events WHERE call_room_id IN (SELECT id FROM call_rooms WHERE host_user_id = ?)", hostID)
		db.Exec("DELETE FROM call_participants WHERE call_room_id IN (SELECT id FROM call_rooms WHERE host_user_id = ?)", hostID)
		db.Exec("DELETE FROM call_guest_tokens WHERE appointment_id = ?", appointment.ID)
		db.Exec("DELETE FROM outbox WHERE aggregate_type = 'call' AND aggregate_id IN (SELECT id FROM call_rooms WHERE host_user_id = ?)", hostID)
		db.Exec("DELETE FROM call_rooms WHERE host_user_id = ?", hostID)
	})
	now := time.Now().UTC().Truncate(time.Microsecond)

	t.Run("one live room per appointment", func(t *testing.T) {
		room, err := rooms.Create(ctx, &models.CallRoom{Kind: models.CallRoomKindAppointment, AppointmentID: &appointment.ID, HostUserID: hostID, MaxParticipants: 2, Status: models.CallRoomStatusOpen, OpenedAt: now})
		if err != nil {
			t.Fatalf("Create appointment room returned error: %v", err)
		}
		if _, err := rooms.Create(ctx, &models.CallRoom{Kind: models.CallRoomKindAppointment, AppointmentID: &appointment.ID, HostUserID: hostID, MaxParticipants: 2, Status: models.CallRoomStatusOpen, OpenedAt: now}); !errors.Is(err, apperrors.ErrCallRoomExists) {
			t.Fatalf("Create second live room error = %v, want %v", err, apperrors.ErrCallRoomExists)
		}

		err = txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
			locked, err := rooms.GetLiveByAppointmentForUpdate(txCtx, appointment.ID)
			if err != nil {
				return err
			}
			if locked.ID != room.ID {
				return fmt.Errorf("locked room %d, want %d", locked.ID, room.ID)
			}
			return rooms.End(txCtx, locked.ID, models.CallEndReasonHangup, now)
		})
		if err != nil {
			t.Fatalf("end live room: %v", err)
		}
		if err := rooms.End(ctx, room.ID, models.CallEndReasonHangup, now); !errors.Is(err, apperrors.ErrCallRoomNotFound) {
			t.Fatalf("End an ended room error = %v, want %v", err, apperrors.ErrCallRoomNotFound)
		}
		ended, err := rooms.GetByID(ctx, room.ID)
		if err != nil || ended.Status != models.CallRoomStatusEnded || ended.EndReason != models.CallEndReasonHangup || ended.EndedAt == nil {
			t.Fatalf("ended room = %+v, %v, want ended/hangup with a time", ended, err)
		}
		if _, err := rooms.GetLiveByAppointment(ctx, appointment.ID); !errors.Is(err, apperrors.ErrCallRoomNotFound) {
			t.Fatalf("GetLiveByAppointment after the end error = %v, want %v", err, apperrors.ErrCallRoomNotFound)
		}
		if _, err := rooms.Create(ctx, &models.CallRoom{Kind: models.CallRoomKindAppointment, AppointmentID: &appointment.ID, HostUserID: hostID, MaxParticipants: 2, Status: models.CallRoomStatusOpen, OpenedAt: now}); err != nil {
			t.Fatalf("Create a room after the end returned error: %v", err)
		}
	})

	t.Run("code rooms", func(t *testing.T) {
		codeHash := fmt.Sprintf("code-%d", time.Now().UnixNano())
		expiresAt := now.Add(time.Hour)
		room, err := rooms.Create(ctx, &models.CallRoom{Kind: models.CallRoomKindCode, HostUserID: hostID, Title: "Family", JoinCodeHash: codeHash, MaxParticipants: 5, RequiresApproval: true, Status: models.CallRoomStatusOpen, OpenedAt: now, ExpiresAt: &expiresAt})
		if err != nil {
			t.Fatalf("Create code room returned error: %v", err)
		}
		if _, err := rooms.Create(ctx, &models.CallRoom{Kind: models.CallRoomKindCode, HostUserID: hostID, JoinCodeHash: codeHash, MaxParticipants: 5, Status: models.CallRoomStatusOpen, OpenedAt: now}); !errors.Is(err, apperrors.ErrCallJoinCodeExists) {
			t.Fatalf("Create a room with a live code error = %v, want %v", err, apperrors.ErrCallJoinCodeExists)
		}
		if _, err := rooms.Create(ctx, &models.CallRoom{Kind: models.CallRoomKindCode, HostUserID: hostID, MaxParticipants: 5, Status: models.CallRoomStatusOpen, OpenedAt: now}); err == nil {
			t.Fatalf("Create a code room without a code returned nil, want a check violation")
		}

		found, err := rooms.GetLiveByJoinCodeHashForUpdate(ctx, codeHash)
		if err != nil || found.ID != room.ID || !found.RequiresApproval || found.Title != "Family" || found.ExpiresAt == nil {
			t.Fatalf("GetLiveByJoinCodeHashForUpdate = %+v, %v, want the room", found, err)
		}
		if live, err := rooms.CountLiveByHost(ctx, hostID, models.CallRoomKindCode); err != nil || live != 1 {
			t.Fatalf("CountLiveByHost = %d, %v, want 1", live, err)
		}
		if created, err := rooms.CountCreatedByHostSince(ctx, hostID, models.CallRoomKindCode, now.Add(-time.Hour)); err != nil || created != 1 {
			t.Fatalf("CountCreatedByHostSince = %d, %v, want 1", created, err)
		}
		if listed, err := rooms.ListByHost(ctx, hostID, models.CallRoomKindCode, 10); err != nil || len(listed) != 1 || listed[0].JoinCodeHash != codeHash {
			t.Fatalf("ListByHost = %v, %v, want the code room", listed, err)
		}
		if live, err := rooms.ListLive(ctx, 1000); err != nil || len(live) < 2 {
			t.Fatalf("ListLive = %d rooms, %v, want both live rooms", len(live), err)
		}

		if started, err := rooms.MarkActive(ctx, room.ID, now); err != nil || !started {
			t.Fatalf("MarkActive = %v, %v, want started", started, err)
		}
		if started, err := rooms.MarkActive(ctx, room.ID, now); err != nil || started {
			t.Fatalf("MarkActive twice = %v, %v, want no change", started, err)
		}

		userID := fixture.booker.ID
		host, err := participants.Create(ctx, &models.CallParticipant{CallRoomID: room.ID, UserID: &userID, PeerID: "peer-host", DisplayName: "Dr. Rao", Role: models.CallRoleHost, AdmissionStatus: models.CallAdmissionAdmitted, MediaMode: models.CallMediaAudio})
		if err != nil {
			t.Fatalf("Create host participant returned error: %v", err)
		}
		if _, err := participants.Create(ctx, &models.CallParticipant{CallRoomID: room.ID, UserID: &userID, PeerID: "peer-again", DisplayName: "Dr. Rao", Role: models.CallRoleHost, AdmissionStatus: models.CallAdmissionAdmitted, MediaMode: models.CallMediaAudio}); err == nil {
			t.Fatalf("Create a second row for the same user returned nil, want a unique violation")
		}
		member, err := participants.Create(ctx, &models.CallParticipant{CallRoomID: room.ID, PeerID: "peer-member", DisplayName: "Ravi", Role: models.CallRoleMember, AdmissionStatus: models.CallAdmissionWaiting, MediaMode: models.CallMediaAudio})
		if err != nil {
			t.Fatalf("Create member participant returned error: %v", err)
		}

		member.AdmissionStatus = models.CallAdmissionAdmitted
		member.MutedByHost = true
		member.ConnectionState = models.CallConnectionRelay
		member.JoinedAt = &now
		member.LastSeenAt = &now
		updated, err := participants.Update(ctx, member)
		if err != nil || updated.AdmissionStatus != models.CallAdmissionAdmitted || !updated.MutedByHost || updated.ConnectionState != models.CallConnectionRelay || updated.LastSeenAt == nil {
			t.Fatalf("Update = %+v, %v, want the changes saved", updated, err)
		}
		later := now.Add(time.Minute)
		if err := participants.Touch(ctx, host.ID, later); err != nil {
			t.Fatalf("Touch returned error: %v", err)
		}
		listed, err := participants.ListByRoom(ctx, room.ID)
		if err != nil || len(listed) != 2 || listed[0].LastSeenAt == nil || !listed[0].LastSeenAt.Equal(later) {
			t.Fatalf("ListByRoom = %v, %v, want both with the host touched", listed, err)
		}

		if err := events.Create(ctx, &models.CallEvent{CallRoomID: room.ID, CallParticipantID: &member.ID, EventType: models.CallEventMuted}); err != nil {
			t.Fatalf("Create participant event returned error: %v", err)
		}
		if err := events.Create(ctx, &models.CallEvent{CallRoomID: room.ID, EventType: models.CallEventEnded, Detail: string(models.CallEndReasonHostEnded)}); err != nil {
			t.Fatalf("Create room event returned error: %v", err)
		}
	})

	t.Run("guest tokens", func(t *testing.T) {
		tokenHash := fmt.Sprintf("guest-%d", time.Now().UnixNano())
		created, err := guestTokens.Create(ctx, &models.CallGuestToken{AppointmentID: appointment.ID, CreatedByUserID: hostID, TokenHash: tokenHash, ExpiresAt: now.Add(time.Hour), MaxUses: 3})
		if err != nil {
			t.Fatalf("Create guest token returned error: %v", err)
		}
		if err := guestTokens.IncrementUse(ctx, created.ID); err != nil {
			t.Fatalf("IncrementUse returned error: %v", err)
		}
		found, err := guestTokens.GetByTokenHashForUpdate(ctx, tokenHash)
		if err != nil || found.UseCount != 1 || found.MaxUses != 3 {
			t.Fatalf("GetByTokenHashForUpdate = %+v, %v, want one use of three", found, err)
		}
		if _, err := guestTokens.GetByTokenHashForUpdate(ctx, "missing"); !errors.Is(err, apperrors.ErrCallGuestLinkNotFound) {
			t.Fatalf("GetByTokenHashForUpdate missing error = %v, want %v", err, apperrors.ErrCallGuestLinkNotFound)
		}
		if err := guestTokens.Revoke(ctx, appointment.ID+1, created.ID, now); !errors.Is(err, apperrors.ErrCallGuestLinkNotFound) {
			t.Fatalf("Revoke through another appointment error = %v, want %v", err, apperrors.ErrCallGuestLinkNotFound)
		}
		if err := guestTokens.Revoke(ctx, appointment.ID, created.ID, now); err != nil {
			t.Fatalf("Revoke returned error: %v", err)
		}
		if err := guestTokens.Revoke(ctx, appointment.ID, created.ID, now); !errors.Is(err, apperrors.ErrCallGuestLinkNotFound) {
			t.Fatalf("Revoke twice error = %v, want %v", err, apperrors.ErrCallGuestLinkNotFound)
		}
	})

	t.Run("join attempts are counted within the window", func(t *testing.T) {
		for _, at := range []time.Time{now, now.Add(-time.Minute), now.Add(-time.Hour)} {
			if err := attempts.Create(ctx, clientIP, at); err != nil {
				t.Fatalf("Create attempt returned error: %v", err)
			}
		}
		if count, err := attempts.CountSince(ctx, clientIP, now.Add(-10*time.Minute)); err != nil || count != 2 {
			t.Fatalf("CountSince = %d, %v, want 2 in the window", count, err)
		}
		if err := attempts.DeleteBefore(ctx, clientIP, now.Add(-10*time.Minute)); err != nil {
			t.Fatalf("DeleteBefore returned error: %v", err)
		}
		if count, err := attempts.CountSince(ctx, clientIP, time.Time{}); err != nil || count != 2 {
			t.Fatalf("CountSince after cleanup = %d, %v, want 2", count, err)
		}
	})
}
