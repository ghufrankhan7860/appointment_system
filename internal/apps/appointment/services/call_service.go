package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/utils"
)

// CallService runs the call of an appointment: its doctor and one patient-side
// peer, who may be a guest the doctor sent a link to. The room is created by
// whoever opens it first and ends on hangup, when the appointment closes, or
// when the expiry job finds it past its window. Media flows peer to peer; this
// service only decides who may join and hands out the signaling token and ICE
// servers.
type CallService struct {
	appointments AppointmentRepository
	doctors      DoctorRepository
	userPatients UserPatientRepository
	users        UserRepository
	rooms        CallRoomRepository
	participants CallParticipantRepository
	guestTokens  CallGuestTokenRepository
	events       CallEventRepository
	attempts     CallJoinAttemptRepository
	outbox       OutboxRepository
	tx           TransactionManager
	tokens       CallTokenIssuer
	hub          CallHub
	ice          ICESettings
	now          func() time.Time
	logger       Logger
}

func NewCallService(
	appointments AppointmentRepository,
	doctors DoctorRepository,
	userPatients UserPatientRepository,
	users UserRepository,
	rooms CallRoomRepository,
	participants CallParticipantRepository,
	guestTokens CallGuestTokenRepository,
	events CallEventRepository,
	attempts CallJoinAttemptRepository,
	outbox OutboxRepository,
	tx TransactionManager,
	tokens CallTokenIssuer,
	hub CallHub,
	ice ICESettings,
	now func() time.Time,
	logger Logger,
) (*CallService, error) {
	if appointments == nil {
		return nil, apperrors.ErrNilAppointmentRepository
	}
	if doctors == nil {
		return nil, apperrors.ErrNilDoctorRepository
	}
	if userPatients == nil {
		return nil, apperrors.ErrNilUserPatientRepository
	}
	if users == nil {
		return nil, apperrors.ErrNilUserRepository
	}
	if rooms == nil {
		return nil, apperrors.ErrNilCallRoomRepository
	}
	if participants == nil {
		return nil, apperrors.ErrNilCallParticipantRepository
	}
	if guestTokens == nil {
		return nil, apperrors.ErrNilCallGuestTokenRepository
	}
	if events == nil {
		return nil, apperrors.ErrNilCallEventRepository
	}
	if attempts == nil {
		return nil, apperrors.ErrNilCallJoinAttemptRepository
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

	return &CallService{
		appointments: appointments,
		doctors:      doctors,
		userPatients: userPatients,
		users:        users,
		rooms:        rooms,
		participants: participants,
		guestTokens:  guestTokens,
		events:       events,
		attempts:     attempts,
		outbox:       outbox,
		tx:           tx,
		tokens:       tokens,
		hub:          hub,
		ice:          ice,
		now:          now,
		logger:       logger,
	}, nil
}

// Open joins the acting user to the appointment's call, creating the room if
// nobody has opened it yet. Only the appointment's doctor and the patient side
// with full or appointments_only access can join, inside the call window of a
// scheduled appointment that is paid or free. Each side has one seat; the same
// user opening again gets the same seat back.
func (cs *CallService) Open(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	appointmentID uint64,
	mediaMode models.CallMediaMode,
) (*models.CallJoin, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("open call context: %w", err)
	}
	if actingUserID == 0 || appointmentID == 0 {
		return nil, fmt.Errorf("open call with missing ids: %w", apperrors.ErrInvalidCallInput)
	}
	mediaMode, err := normalizeMediaMode(mediaMode)
	if err != nil {
		return nil, err
	}

	appointment, role, err := cs.access(ctx, actingUserID, actingRole, appointmentID)
	if err != nil {
		return nil, err
	}
	now := cs.now()
	if err := checkCallWindow(&appointment.Appointment, now); err != nil {
		return nil, err
	}
	user, err := cs.users.GetByID(ctx, actingUserID)
	if err != nil {
		return nil, fmt.Errorf("get acting user: %w", err)
	}

	for try := 1; ; try++ {
		join, err := cs.open(ctx, appointmentID, actingUserID, role, user.Name, mediaMode, now)
		if errors.Is(err, apperrors.ErrCallRoomExists) && try < constant.MaxCallRoomOpenTries {
			continue
		}
		if err != nil {
			return nil, err
		}

		cs.logger.InfoContext(ctx, "call opened", "appointment_id", appointmentID, "call_room_id", join.Room.ID, "role", role)

		return join, nil
	}
}

// Get returns the appointment's live room with who is in it.
func (cs *CallService) Get(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	appointmentID uint64,
) (*models.CallRoomState, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("get call context: %w", err)
	}
	if actingUserID == 0 || appointmentID == 0 {
		return nil, fmt.Errorf("get call with missing ids: %w", apperrors.ErrInvalidCallInput)
	}

	if _, _, err := cs.access(ctx, actingUserID, actingRole, appointmentID); err != nil {
		return nil, err
	}

	room, err := cs.rooms.GetLiveByAppointment(ctx, appointmentID)
	if err != nil {
		if errors.Is(err, apperrors.ErrCallRoomNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("get live call room: %w", err)
	}
	participants, err := cs.participants.ListByRoom(ctx, room.ID)
	if err != nil {
		return nil, fmt.Errorf("list call participants: %w", err)
	}

	return callRoomState(room, participants, cs.now()), nil
}

// End hangs up the appointment's call for everyone in it.
func (cs *CallService) End(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	appointmentID uint64,
) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("end call context: %w", err)
	}
	if actingUserID == 0 || appointmentID == 0 {
		return fmt.Errorf("end call with missing ids: %w", apperrors.ErrInvalidCallInput)
	}

	if _, _, err := cs.access(ctx, actingUserID, actingRole, appointmentID); err != nil {
		return err
	}

	var roomID uint64
	err := cs.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		room, err := cs.rooms.GetLiveByAppointmentForUpdate(txCtx, appointmentID)
		if err != nil {
			return err
		}
		roomID = room.ID

		return endCallRoom(txCtx, cs.rooms, cs.events, cs.outbox, room, models.CallEndReasonHangup, actingUserID, cs.now())
	})
	if err != nil {
		if errors.Is(err, apperrors.ErrCallRoomNotFound) {
			return err
		}
		return fmt.Errorf("end call: %w", err)
	}

	cs.hub.CloseRoom(roomID, models.CallSignal{Type: models.CallSignalEnded, Reason: string(models.CallEndReasonHangup)})
	cs.logger.InfoContext(ctx, "call ended", "appointment_id", appointmentID, "call_room_id", roomID, "end_reason", models.CallEndReasonHangup)

	return nil
}

// CreateGuestLink lets the appointment's doctor invite someone without an
// account to the call as the patient side. It returns the link with its token,
// which is shown only this once. The link works until the call window closes,
// up to maxUses joins (0 means constant.DefaultGuestLinkUses).
func (cs *CallService) CreateGuestLink(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	appointmentID uint64,
	displayName string,
	maxUses int,
) (*models.CallGuestToken, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", fmt.Errorf("create call guest link context: %w", err)
	}
	if actingUserID == 0 || appointmentID == 0 {
		return nil, "", fmt.Errorf("create call guest link with missing ids: %w", apperrors.ErrInvalidCallInput)
	}
	displayName, err := normalizeDisplayName(displayName, false)
	if err != nil {
		return nil, "", err
	}
	if maxUses == 0 {
		maxUses = constant.DefaultGuestLinkUses
	}
	if maxUses < 1 || maxUses > constant.MaxGuestLinkUses {
		return nil, "", apperrors.Invalid(apperrors.ErrInvalidCallInput, "maxUses", fmt.Sprintf("must be between 1 and %d", constant.MaxGuestLinkUses))
	}

	appointment, err := cs.doctorAccess(ctx, actingUserID, actingRole, appointmentID)
	if err != nil {
		return nil, "", err
	}
	expiresAt := appointment.EndsAt.Add(constant.CallJoinGrace)
	if appointment.Status != models.AppointmentStatusScheduled {
		return nil, "", apperrors.Invalid(apperrors.ErrCallNotOpen, "", fmt.Sprintf("appointment is %s; only a scheduled appointment has a call", appointment.Status))
	}
	if !cs.now().Before(expiresAt) {
		return nil, "", apperrors.Invalid(apperrors.ErrCallNotOpen, "", fmt.Sprintf("the call closed %d minutes after the appointment", int(constant.CallJoinGrace.Minutes())))
	}

	token, err := utils.NewOpaqueToken()
	if err != nil {
		return nil, "", fmt.Errorf("generate call guest token: %w", err)
	}
	created, err := cs.guestTokens.Create(ctx, &models.CallGuestToken{
		AppointmentID:   appointmentID,
		CreatedByUserID: actingUserID,
		TokenHash:       utils.HashToken(token),
		DisplayName:     displayName,
		ExpiresAt:       expiresAt,
		MaxUses:         maxUses,
	})
	if err != nil {
		return nil, "", fmt.Errorf("create call guest link: %w", err)
	}

	cs.logger.InfoContext(ctx, "call guest link created", "appointment_id", appointmentID, "guest_link_id", created.ID, "max_uses", maxUses)

	return created, token, nil
}

func (cs *CallService) RevokeGuestLink(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	appointmentID uint64,
	linkID uint64,
) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("revoke call guest link context: %w", err)
	}
	if actingUserID == 0 || appointmentID == 0 || linkID == 0 {
		return fmt.Errorf("revoke call guest link with missing ids: %w", apperrors.ErrInvalidCallInput)
	}

	if _, err := cs.doctorAccess(ctx, actingUserID, actingRole, appointmentID); err != nil {
		return err
	}

	if err := cs.guestTokens.Revoke(ctx, appointmentID, linkID, cs.now()); err != nil {
		if errors.Is(err, apperrors.ErrCallGuestLinkNotFound) {
			return err
		}
		return fmt.Errorf("revoke call guest link: %w", err)
	}

	cs.logger.InfoContext(ctx, "call guest link revoked", "appointment_id", appointmentID, "guest_link_id", linkID)

	return nil
}

// JoinAsGuest joins someone without an account to an appointment's call with
// a guest link. An unknown, revoked, expired or used-up link is one
// ErrCallJoinInvalid and counts as a failed attempt from clientIP; after
// constant.MaxFailedCallJoinAttempts within constant.CallJoinAttemptWindow it
// answers ErrCallJoinAttemptsExceeded until the window passes. The guest takes
// the patient side's seat, so it is refused while the patient side is in.
func (cs *CallService) JoinAsGuest(
	ctx context.Context,
	clientIP string,
	token string,
	displayName string,
	mediaMode models.CallMediaMode,
) (*models.CallJoin, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("join call as guest context: %w", err)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, apperrors.Invalid(apperrors.ErrInvalidCallInput, "token", "is required")
	}
	displayName, err := normalizeDisplayName(displayName, false)
	if err != nil {
		return nil, err
	}
	mediaMode, err = normalizeMediaMode(mediaMode)
	if err != nil {
		return nil, err
	}

	now := cs.now()
	if err := checkCallJoinAttempts(ctx, cs.attempts, clientIP, now); err != nil {
		return nil, err
	}

	var room *models.CallRoom
	var participant *models.CallParticipant
	err = cs.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		link, err := cs.guestTokens.GetByTokenHashForUpdate(txCtx, utils.HashToken(token))
		if err != nil {
			if errors.Is(err, apperrors.ErrCallGuestLinkNotFound) {
				return apperrors.ErrCallJoinInvalid
			}
			return fmt.Errorf("get call guest link: %w", err)
		}
		if link.RevokedAt != nil || !now.Before(link.ExpiresAt) || link.UseCount >= link.MaxUses {
			return apperrors.ErrCallJoinInvalid
		}
		if displayName == "" {
			displayName = link.DisplayName
		}
		if displayName == "" {
			return apperrors.Invalid(apperrors.ErrInvalidCallInput, "displayName", "is required")
		}

		appointment, err := cs.appointments.GetByID(txCtx, link.AppointmentID)
		if err != nil {
			return fmt.Errorf("get appointment of call guest link: %w", err)
		}
		if err := checkCallWindow(&appointment.Appointment, now); err != nil {
			return err
		}

		room, err = cs.liveRoom(txCtx, link.AppointmentID, link.CreatedByUserID, now)
		if err != nil {
			return err
		}
		participants, err := cs.participants.ListByRoom(txCtx, room.ID)
		if err != nil {
			return fmt.Errorf("list call participants: %w", err)
		}
		if !appointmentSeatFree(participants, 0, models.CallRoleGuest, now) {
			return apperrors.ErrCallRoomFull
		}

		peerID, err := newPeerID()
		if err != nil {
			return err
		}
		participant, err = cs.participants.Create(txCtx, &models.CallParticipant{
			CallRoomID:      room.ID,
			GuestTokenID:    &link.ID,
			PeerID:          peerID,
			DisplayName:     displayName,
			Role:            models.CallRoleGuest,
			AdmissionStatus: models.CallAdmissionAdmitted,
			MediaMode:       mediaMode,
		})
		if err != nil {
			return fmt.Errorf("create call guest participant: %w", err)
		}

		return cs.guestTokens.IncrementUse(txCtx, link.ID)
	})
	if err != nil {
		if errors.Is(err, apperrors.ErrCallJoinInvalid) {
			if recordErr := recordFailedCallJoin(ctx, cs.attempts, clientIP, now); recordErr != nil {
				return nil, recordErr
			}
			return nil, err
		}
		if errors.Is(err, apperrors.ErrInvalidCallInput) ||
			errors.Is(err, apperrors.ErrCallNotOpen) ||
			errors.Is(err, apperrors.ErrCallRoomFull) {
			return nil, err
		}
		return nil, fmt.Errorf("join call as guest: %w", err)
	}

	cs.logger.InfoContext(ctx, "call joined as guest", "call_room_id", room.ID, "call_participant_id", participant.ID)

	return issueCallJoin(cs.tokens, cs.ice, room, participant, now)
}

// Expire ends at most limit live rooms whose time is over: an appointment room
// once its appointment is closed or its call window has passed (no_answer if
// the second peer never came), a code room past its expiry, left by its host
// for constant.CodeRoomHostGrace, or empty for constant.CodeRoomIdleTimeout.
// It returns how many rooms it ended.
func (cs *CallService) Expire(ctx context.Context, limit int) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("expire call rooms context: %w", err)
	}

	rooms, err := cs.rooms.ListLive(ctx, limit)
	if err != nil {
		return 0, fmt.Errorf("list live call rooms: %w", err)
	}

	ended := 0
	for _, room := range rooms {
		if err := ctx.Err(); err != nil {
			return ended, fmt.Errorf("expire call rooms context: %w", err)
		}

		now := cs.now()
		reason, err := cs.expiryReason(ctx, room, now)
		if err != nil {
			return ended, err
		}
		if reason == "" {
			continue
		}

		err = cs.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
			locked, err := cs.rooms.GetByIDForUpdate(txCtx, room.ID)
			if err != nil {
				return fmt.Errorf("lock call room: %w", err)
			}

			return endCallRoom(txCtx, cs.rooms, cs.events, cs.outbox, locked, reason, 0, now)
		})
		if errors.Is(err, apperrors.ErrCallRoomNotFound) {
			continue
		}
		if err != nil {
			return ended, fmt.Errorf("expire call room %d: %w", room.ID, err)
		}

		cs.hub.CloseRoom(room.ID, models.CallSignal{Type: models.CallSignalEnded, Reason: string(reason)})
		cs.logger.InfoContext(ctx, "call room expired", "call_room_id", room.ID, "end_reason", reason)
		ended++
	}

	return ended, nil
}

func (cs *CallService) open(
	ctx context.Context,
	appointmentID uint64,
	actingUserID uint64,
	role models.CallParticipantRole,
	displayName string,
	mediaMode models.CallMediaMode,
	now time.Time,
) (*models.CallJoin, error) {
	var room *models.CallRoom
	var participant *models.CallParticipant
	err := cs.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		room, err = cs.liveRoom(txCtx, appointmentID, actingUserID, now)
		if err != nil {
			return err
		}
		participants, err := cs.participants.ListByRoom(txCtx, room.ID)
		if err != nil {
			return fmt.Errorf("list call participants: %w", err)
		}

		existing := findCallParticipant(participants, func(candidate *models.CallParticipant) bool {
			return candidate.UserID != nil && *candidate.UserID == actingUserID
		})
		var existingID uint64
		if existing != nil {
			existingID = existing.ID
		}
		if !appointmentSeatFree(participants, existingID, role, now) {
			return apperrors.ErrCallRoomFull
		}

		if existing != nil {
			existing.MediaMode = mediaMode
			participant, err = cs.participants.Update(txCtx, existing)
			if err != nil {
				return fmt.Errorf("update call participant: %w", err)
			}
			return nil
		}

		peerID, err := newPeerID()
		if err != nil {
			return err
		}
		participant, err = cs.participants.Create(txCtx, &models.CallParticipant{
			CallRoomID:      room.ID,
			UserID:          &actingUserID,
			PeerID:          peerID,
			DisplayName:     displayName,
			Role:            role,
			AdmissionStatus: models.CallAdmissionAdmitted,
			MediaMode:       mediaMode,
		})
		if err != nil {
			return fmt.Errorf("create call participant: %w", err)
		}

		return nil
	})
	if err != nil {
		if errors.Is(err, apperrors.ErrCallRoomFull) || errors.Is(err, apperrors.ErrCallRoomExists) {
			return nil, err
		}
		return nil, fmt.Errorf("open call: %w", err)
	}

	return issueCallJoin(cs.tokens, cs.ice, room, participant, now)
}

// liveRoom returns the appointment's live room locked for the transaction,
// creating it when there is none. A concurrent create makes it return
// ErrCallRoomExists, and the caller tries again.
func (cs *CallService) liveRoom(txCtx context.Context, appointmentID uint64, openedByUserID uint64, now time.Time) (*models.CallRoom, error) {
	room, err := cs.rooms.GetLiveByAppointmentForUpdate(txCtx, appointmentID)
	if err == nil {
		return room, nil
	}
	if !errors.Is(err, apperrors.ErrCallRoomNotFound) {
		return nil, fmt.Errorf("get live call room: %w", err)
	}

	room, err = cs.rooms.Create(txCtx, &models.CallRoom{
		Kind:            models.CallRoomKindAppointment,
		AppointmentID:   &appointmentID,
		HostUserID:      openedByUserID,
		MaxParticipants: constant.AppointmentCallMaxParticipants,
		Status:          models.CallRoomStatusOpen,
		OpenedAt:        now,
	})
	if err != nil {
		if errors.Is(err, apperrors.ErrCallRoomExists) {
			return nil, err
		}
		return nil, fmt.Errorf("create call room: %w", err)
	}
	if err := cs.events.Create(txCtx, callEvent(room.ID, 0, models.CallEventOpened, "")); err != nil {
		return nil, fmt.Errorf("create call opened event: %w", err)
	}

	return room, nil
}

func (cs *CallService) access(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	appointmentID uint64,
) (*models.AppointmentDetails, models.CallParticipantRole, error) {
	appointment, err := cs.appointments.GetByID(ctx, appointmentID)
	if err != nil {
		if errors.Is(err, apperrors.ErrAppointmentNotFound) {
			return nil, "", err
		}
		return nil, "", fmt.Errorf("get appointment by id: %w", err)
	}

	role, err := callRole(ctx, cs.doctors, cs.userPatients, actingUserID, actingRole, appointment)
	if err != nil {
		return nil, "", err
	}

	return appointment, role, nil
}

// doctorAccess allows only the appointment's doctor.
func (cs *CallService) doctorAccess(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	appointmentID uint64,
) (*models.AppointmentDetails, error) {
	appointment, role, err := cs.access(ctx, actingUserID, actingRole, appointmentID)
	if err != nil {
		return nil, err
	}
	if role != models.CallRoleDoctor {
		return nil, apperrors.ErrAppointmentNotFound
	}

	return appointment, nil
}

func (cs *CallService) expiryReason(ctx context.Context, room *models.CallRoom, now time.Time) (models.CallEndReason, error) {
	if room.Kind == models.CallRoomKindAppointment {
		appointment, err := cs.appointments.GetByID(ctx, *room.AppointmentID)
		if err != nil {
			return "", fmt.Errorf("get appointment of call room %d: %w", room.ID, err)
		}
		switch {
		case appointment.Status == models.AppointmentStatusCompleted ||
			appointment.Status == models.AppointmentStatusCancelled ||
			appointment.Status == models.AppointmentStatusNoShow:
			return models.CallEndReasonAppointmentClosed, nil
		case now.Before(appointment.EndsAt.Add(constant.CallJoinGrace)):
			return "", nil
		case room.StartedAt == nil:
			return models.CallEndReasonNoAnswer, nil
		default:
			return models.CallEndReasonTimeout, nil
		}
	}

	if room.ExpiresAt != nil && !now.Before(*room.ExpiresAt) {
		return models.CallEndReasonExpired, nil
	}

	participants, err := cs.participants.ListByRoom(ctx, room.ID)
	if err != nil {
		return "", fmt.Errorf("list participants of call room %d: %w", room.ID, err)
	}
	lastActive := room.OpenedAt
	anyoneIn := false
	for _, participant := range participants {
		if inRoom(participant, now) {
			anyoneIn = true
		}
		seen := lastSeen(participant)
		if participant.Role == models.CallRoleHost && participant.JoinedAt != nil &&
			!connected(participant, now) && now.Sub(seen) >= constant.CodeRoomHostGrace {
			return models.CallEndReasonHostEnded, nil
		}
		if seen.After(lastActive) {
			lastActive = seen
		}
	}
	if !anyoneIn && now.Sub(lastActive) >= constant.CodeRoomIdleTimeout {
		return models.CallEndReasonTimeout, nil
	}

	return "", nil
}

// lastSeen is when the participant was last known to be in the room.
func lastSeen(participant *models.CallParticipant) time.Time {
	var seen time.Time
	if participant.LastSeenAt != nil {
		seen = *participant.LastSeenAt
	}
	if participant.LeftAt != nil && participant.LeftAt.After(seen) {
		seen = *participant.LeftAt
	}

	return seen
}
