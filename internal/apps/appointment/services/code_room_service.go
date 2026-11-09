package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/utils"
)

// CodeRoomService runs rooms a verified doctor hosts outside any appointment:
// a family briefing, a follow-up group, a second opinion. Anyone with the code
// can join under a display name, by default into a lobby the host admits from.
// A code room is not a medical record and grants no access to the host's
// appointments or records. Only the code's hash is stored, and wrong codes are
// capped per IP.
type CodeRoomService struct {
	doctors      DoctorRepository
	users        UserRepository
	rooms        CallRoomRepository
	participants CallParticipantRepository
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

func NewCodeRoomService(
	doctors DoctorRepository,
	users UserRepository,
	rooms CallRoomRepository,
	participants CallParticipantRepository,
	events CallEventRepository,
	attempts CallJoinAttemptRepository,
	outbox OutboxRepository,
	tx TransactionManager,
	tokens CallTokenIssuer,
	hub CallHub,
	ice ICESettings,
	now func() time.Time,
	logger Logger,
) (*CodeRoomService, error) {
	if doctors == nil {
		return nil, apperrors.ErrNilDoctorRepository
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

	return &CodeRoomService{
		doctors:      doctors,
		users:        users,
		rooms:        rooms,
		participants: participants,
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

// Create opens a code room and returns it with its join code, which is shown
// only this once. A zero maxParticipants means constant.MaxCodeRoomParticipants
// and a nil requiresApproval means a lobby. With scheduledAt the room opens
// constant.CallJoinLeadTime before it and lives until
// constant.CodeRoomScheduledTTL after; without it, it lives
// constant.CodeRoomTTL. A host keeps at most constant.MaxLiveCodeRoomsPerHost
// live rooms and creates at most constant.MaxCodeRoomsPerDay a day.
func (crs *CodeRoomService) Create(
	ctx context.Context,
	actingUserID uint64,
	title string,
	maxParticipants int,
	requiresApproval *bool,
	scheduledAt *time.Time,
) (*models.CallRoom, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", fmt.Errorf("create code room context: %w", err)
	}
	if actingUserID == 0 {
		return nil, "", fmt.Errorf("create code room without acting user: %w", apperrors.ErrInvalidCallInput)
	}

	title = strings.TrimSpace(title)
	if utf8.RuneCountInString(title) > constant.MaxCallTitleLength {
		return nil, "", apperrors.Invalid(apperrors.ErrInvalidCallInput, "title", fmt.Sprintf("must be at most %d characters", constant.MaxCallTitleLength))
	}
	if maxParticipants == 0 {
		maxParticipants = constant.MaxCodeRoomParticipants
	}
	if maxParticipants < constant.MinCodeRoomParticipants || maxParticipants > constant.MaxCodeRoomParticipants {
		return nil, "", apperrors.Invalid(apperrors.ErrInvalidCallInput, "maxParticipants", fmt.Sprintf("must be between %d and %d", constant.MinCodeRoomParticipants, constant.MaxCodeRoomParticipants))
	}
	lobby := requiresApproval == nil || *requiresApproval

	now := crs.now()
	expiresAt := now.Add(constant.CodeRoomTTL)
	if scheduledAt != nil {
		if !scheduledAt.After(now) {
			return nil, "", apperrors.Invalid(apperrors.ErrInvalidCallInput, "scheduledAt", "must be in the future")
		}
		if scheduledAt.After(now.Add(constant.MaxCodeRoomScheduleAhead)) {
			return nil, "", apperrors.Invalid(apperrors.ErrInvalidCallInput, "scheduledAt", fmt.Sprintf("must be within %d days", int(constant.MaxCodeRoomScheduleAhead.Hours()/24)))
		}
		expiresAt = scheduledAt.Add(constant.CodeRoomScheduledTTL)
	}

	if err := crs.verifiedHost(ctx, actingUserID); err != nil {
		return nil, "", err
	}
	live, err := crs.rooms.CountLiveByHost(ctx, actingUserID, models.CallRoomKindCode)
	if err != nil {
		return nil, "", fmt.Errorf("count live code rooms: %w", err)
	}
	if live >= constant.MaxLiveCodeRoomsPerHost {
		return nil, "", apperrors.Invalid(apperrors.ErrCallRoomLimitReached, "", fmt.Sprintf("you can keep at most %d rooms open; end one first", constant.MaxLiveCodeRoomsPerHost))
	}
	created, err := crs.rooms.CountCreatedByHostSince(ctx, actingUserID, models.CallRoomKindCode, now.Add(-constant.CodeRoomDailyWindow))
	if err != nil {
		return nil, "", fmt.Errorf("count code rooms created today: %w", err)
	}
	if created >= constant.MaxCodeRoomsPerDay {
		return nil, "", apperrors.Invalid(apperrors.ErrCallRoomLimitReached, "", fmt.Sprintf("you can create at most %d rooms a day", constant.MaxCodeRoomsPerDay))
	}

	for try := 1; ; try++ {
		room, code, err := crs.create(ctx, actingUserID, title, maxParticipants, lobby, scheduledAt, expiresAt, now)
		if errors.Is(err, apperrors.ErrCallJoinCodeExists) && try < constant.MaxCodeRoomCodeTries {
			continue
		}
		if err != nil {
			return nil, "", err
		}

		crs.logger.InfoContext(ctx, "code room created", "call_room_id", room.ID, "max_participants", maxParticipants, "requires_approval", lobby)

		return room, code, nil
	}
}

// List returns the host's code rooms, newest first.
func (crs *CodeRoomService) List(ctx context.Context, actingUserID uint64) ([]*models.CallRoom, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list code rooms context: %w", err)
	}
	if actingUserID == 0 {
		return nil, fmt.Errorf("list code rooms without acting user: %w", apperrors.ErrInvalidCallInput)
	}

	rooms, err := crs.rooms.ListByHost(ctx, actingUserID, models.CallRoomKindCode, constant.CallRoomListLimit)
	if err != nil {
		return nil, fmt.Errorf("list code rooms: %w", err)
	}

	return rooms, nil
}

// End ends a live code room for everyone in it.
func (crs *CodeRoomService) End(ctx context.Context, actingUserID uint64, roomID uint64) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("end code room context: %w", err)
	}
	if actingUserID == 0 || roomID == 0 {
		return fmt.Errorf("end code room with missing ids: %w", apperrors.ErrInvalidCallInput)
	}

	err := crs.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		room, err := crs.rooms.GetByIDForUpdate(txCtx, roomID)
		if err != nil {
			return err
		}
		if !hostedBy(room, actingUserID) {
			return apperrors.ErrCallRoomNotFound
		}

		return endCallRoom(txCtx, crs.rooms, crs.events, crs.outbox, room, models.CallEndReasonHostEnded, actingUserID, crs.now())
	})
	if err != nil {
		if errors.Is(err, apperrors.ErrCallRoomNotFound) {
			return err
		}
		return fmt.Errorf("end code room: %w", err)
	}

	crs.hub.CloseRoom(roomID, models.CallSignal{Type: models.CallSignalEnded, Reason: string(models.CallEndReasonHostEnded)})
	crs.logger.InfoContext(ctx, "code room ended", "call_room_id", roomID)

	return nil
}

// ListParticipants returns a host's room with everyone who joined it.
func (crs *CodeRoomService) ListParticipants(ctx context.Context, actingUserID uint64, roomID uint64) (*models.CallRoomState, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list code room participants context: %w", err)
	}
	if actingUserID == 0 || roomID == 0 {
		return nil, fmt.Errorf("list code room participants with missing ids: %w", apperrors.ErrInvalidCallInput)
	}

	room, err := crs.rooms.GetByID(ctx, roomID)
	if err != nil {
		if errors.Is(err, apperrors.ErrCallRoomNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("get code room: %w", err)
	}
	if !hostedBy(room, actingUserID) {
		return nil, apperrors.ErrCallRoomNotFound
	}
	participants, err := crs.participants.ListByRoom(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("list code room participants: %w", err)
	}

	return callRoomState(room, participants, crs.now()), nil
}

// JoinAsHost joins the host to their own live room. The host's seat is always
// free.
func (crs *CodeRoomService) JoinAsHost(
	ctx context.Context,
	actingUserID uint64,
	roomID uint64,
	mediaMode models.CallMediaMode,
) (*models.CallJoin, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("join code room as host context: %w", err)
	}
	if actingUserID == 0 || roomID == 0 {
		return nil, fmt.Errorf("join code room as host with missing ids: %w", apperrors.ErrInvalidCallInput)
	}
	mediaMode, err := normalizeMediaMode(mediaMode)
	if err != nil {
		return nil, err
	}

	if err := crs.verifiedHost(ctx, actingUserID); err != nil {
		return nil, err
	}
	user, err := crs.users.GetByID(ctx, actingUserID)
	if err != nil {
		return nil, fmt.Errorf("get acting user: %w", err)
	}

	now := crs.now()
	var room *models.CallRoom
	var participant *models.CallParticipant
	err = crs.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		room, err = crs.rooms.GetByIDForUpdate(txCtx, roomID)
		if err != nil {
			return err
		}
		if !hostedBy(room, actingUserID) || room.Status == models.CallRoomStatusEnded {
			return apperrors.ErrCallRoomNotFound
		}
		participants, err := crs.participants.ListByRoom(txCtx, roomID)
		if err != nil {
			return fmt.Errorf("list code room participants: %w", err)
		}

		existing := findCallParticipant(participants, func(candidate *models.CallParticipant) bool {
			return candidate.UserID != nil && *candidate.UserID == actingUserID
		})
		if existing != nil {
			existing.MediaMode = mediaMode
			participant, err = crs.participants.Update(txCtx, existing)
			if err != nil {
				return fmt.Errorf("update code room host: %w", err)
			}
			return nil
		}

		peerID, err := newPeerID()
		if err != nil {
			return err
		}
		participant, err = crs.participants.Create(txCtx, &models.CallParticipant{
			CallRoomID:      roomID,
			UserID:          &actingUserID,
			PeerID:          peerID,
			DisplayName:     user.Name,
			Role:            models.CallRoleHost,
			AdmissionStatus: models.CallAdmissionAdmitted,
			MediaMode:       mediaMode,
		})
		if err != nil {
			return fmt.Errorf("create code room host: %w", err)
		}

		return nil
	})
	if err != nil {
		if errors.Is(err, apperrors.ErrCallRoomNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("join code room as host: %w", err)
	}

	crs.logger.InfoContext(ctx, "code room joined by host", "call_room_id", roomID)

	return issueCallJoin(crs.tokens, crs.ice, room, participant, now)
}

// Join joins someone to a code room with its code. A code with a wrong check
// digit is a typo and is refused without counting; an unknown code, or one of
// an expired room, is one ErrCallJoinInvalid and counts as a failed attempt
// from clientIP. With a lobby the joiner waits for the host, and the room's
// size is checked on admission; without one the joiner is admitted while the
// room has space.
func (crs *CodeRoomService) Join(
	ctx context.Context,
	clientIP string,
	code string,
	displayName string,
	mediaMode models.CallMediaMode,
) (*models.CallJoin, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("join code room context: %w", err)
	}
	code, err := normalizeJoinCode(code)
	if err != nil {
		return nil, err
	}
	displayName, err = normalizeDisplayName(displayName, true)
	if err != nil {
		return nil, err
	}
	mediaMode, err = normalizeMediaMode(mediaMode)
	if err != nil {
		return nil, err
	}

	now := crs.now()
	if err := checkCallJoinAttempts(ctx, crs.attempts, clientIP, now); err != nil {
		return nil, err
	}

	var room *models.CallRoom
	var participant *models.CallParticipant
	err = crs.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		room, err = crs.rooms.GetLiveByJoinCodeHashForUpdate(txCtx, utils.HashToken(code))
		if err != nil {
			if errors.Is(err, apperrors.ErrCallRoomNotFound) {
				return apperrors.ErrCallJoinInvalid
			}
			return fmt.Errorf("get code room by code: %w", err)
		}
		if room.ExpiresAt != nil && !now.Before(*room.ExpiresAt) {
			return apperrors.ErrCallJoinInvalid
		}
		if room.ScheduledAt != nil && now.Before(room.ScheduledAt.Add(-constant.CallJoinLeadTime)) {
			return apperrors.Invalid(apperrors.ErrCallNotOpen, "", fmt.Sprintf("the room opens %d minutes before its scheduled time", int(constant.CallJoinLeadTime.Minutes())))
		}

		admission := models.CallAdmissionWaiting
		if !room.RequiresApproval {
			participants, err := crs.participants.ListByRoom(txCtx, room.ID)
			if err != nil {
				return fmt.Errorf("list code room participants: %w", err)
			}
			if !codeRoomMemberFits(room, participants, 0, now) {
				return apperrors.ErrCallRoomFull
			}
			admission = models.CallAdmissionAdmitted
		}

		peerID, err := newPeerID()
		if err != nil {
			return err
		}
		participant, err = crs.participants.Create(txCtx, &models.CallParticipant{
			CallRoomID:      room.ID,
			PeerID:          peerID,
			DisplayName:     displayName,
			Role:            models.CallRoleMember,
			AdmissionStatus: admission,
			MediaMode:       mediaMode,
		})
		if err != nil {
			return fmt.Errorf("create code room member: %w", err)
		}

		return nil
	})
	if err != nil {
		if errors.Is(err, apperrors.ErrCallJoinInvalid) {
			if recordErr := recordFailedCallJoin(ctx, crs.attempts, clientIP, now); recordErr != nil {
				return nil, recordErr
			}
			return nil, err
		}
		if errors.Is(err, apperrors.ErrCallNotOpen) || errors.Is(err, apperrors.ErrCallRoomFull) {
			return nil, err
		}
		return nil, fmt.Errorf("join code room: %w", err)
	}

	crs.logger.InfoContext(ctx, "code room joined", "call_room_id", room.ID, "call_participant_id", participant.ID, "admission_status", participant.AdmissionStatus)

	return issueCallJoin(crs.tokens, crs.ice, room, participant, now)
}

func (crs *CodeRoomService) create(
	ctx context.Context,
	actingUserID uint64,
	title string,
	maxParticipants int,
	lobby bool,
	scheduledAt *time.Time,
	expiresAt time.Time,
	now time.Time,
) (*models.CallRoom, string, error) {
	digits, err := utils.NewCode(constant.CodeRoomCodeAlphabet, constant.CodeRoomCodeDigits)
	if err != nil {
		return nil, "", fmt.Errorf("generate code room code: %w", err)
	}
	check, err := utils.DammCheckDigit(digits)
	if err != nil {
		return nil, "", fmt.Errorf("generate code room check digit: %w", err)
	}
	code := digits + string(check)

	var room *models.CallRoom
	err = crs.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		room, err = crs.rooms.Create(txCtx, &models.CallRoom{
			Kind:             models.CallRoomKindCode,
			HostUserID:       actingUserID,
			Title:            title,
			JoinCodeHash:     utils.HashToken(code),
			MaxParticipants:  maxParticipants,
			RequiresApproval: lobby,
			ScheduledAt:      scheduledAt,
			Status:           models.CallRoomStatusOpen,
			OpenedAt:         now,
			ExpiresAt:        &expiresAt,
		})
		if err != nil {
			return err
		}

		return crs.events.Create(txCtx, callEvent(room.ID, 0, models.CallEventOpened, ""))
	})
	if err != nil {
		if errors.Is(err, apperrors.ErrCallJoinCodeExists) {
			return nil, "", err
		}
		return nil, "", fmt.Errorf("create code room: %w", err)
	}

	return room, code, nil
}

func (crs *CodeRoomService) verifiedHost(ctx context.Context, actingUserID uint64) error {
	doctor, err := crs.doctors.GetByUserID(ctx, actingUserID)
	if err != nil {
		if errors.Is(err, apperrors.ErrDoctorNotFound) {
			return apperrors.ErrCallHostNotAllowed
		}
		return fmt.Errorf("get own doctor profile: %w", err)
	}
	if doctor.VerificationStatus != models.VerificationStatusVerified {
		return apperrors.ErrCallHostNotAllowed
	}

	return nil
}

func hostedBy(room *models.CallRoom, userID uint64) bool {
	return room.Kind == models.CallRoomKindCode && room.HostUserID == userID
}
