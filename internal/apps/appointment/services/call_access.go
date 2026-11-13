package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/utils"
)

// ICESettings are the STUN and TURN servers handed to call peers. TURN is
// left out while TURNURLs or TURNSecret is empty, so calls fall back to STUN.
type ICESettings struct {
	STUNURLs   []string
	TURNURLs   []string
	TURNSecret string
}

// callRole returns the seat the acting user takes in an appointment's call:
// the appointment's doctor, or the patient side with full or appointments_only
// access. Anyone else, read_only included, gets ErrAppointmentNotFound.
func callRole(
	ctx context.Context,
	doctors DoctorRepository,
	userPatients UserPatientRepository,
	actingUserID uint64,
	actingRole models.Role,
	appointment *models.AppointmentDetails,
) (models.CallParticipantRole, error) {
	if actingRole == models.RoleDoctor {
		doctor, err := doctors.GetByUserID(ctx, actingUserID)
		if err != nil && !errors.Is(err, apperrors.ErrDoctorNotFound) {
			return "", fmt.Errorf("get own doctor profile: %w", err)
		}
		if err == nil && doctor.ID == appointment.DoctorID {
			return models.CallRoleDoctor, nil
		}
	}

	link, err := userPatients.GetActiveByUserAndPatient(ctx, actingUserID, appointment.PatientID)
	if err != nil {
		if errors.Is(err, apperrors.ErrUserPatientNotFound) {
			return "", apperrors.ErrAppointmentNotFound
		}
		return "", fmt.Errorf("authorize call access: %w", err)
	}
	if !canBook(link) {
		return "", apperrors.ErrAppointmentNotFound
	}

	return models.CallRolePatientSide, nil
}

// checkCallWindow allows a call only for a scheduled appointment that is paid,
// free or due at the clinic, from constant.CallJoinLeadTime before its start until
// constant.CallJoinGrace after its end.
func checkCallWindow(appointment *models.Appointment, now time.Time) error {
	if appointment.Status != models.AppointmentStatusScheduled {
		return apperrors.Invalid(apperrors.ErrCallNotOpen, "", fmt.Sprintf("appointment is %s; only a scheduled appointment has a call", appointment.Status))
	}
	if appointment.PaymentStatus != models.AppointmentPaymentPaid && appointment.PaymentStatus != models.AppointmentPaymentWaived &&
		appointment.PaymentStatus != models.AppointmentPaymentDueAtClinic {
		return apperrors.Invalid(apperrors.ErrCallNotOpen, "", fmt.Sprintf("payment is %s for this appointment", appointment.PaymentStatus))
	}
	if now.Before(appointment.StartsAt.Add(-constant.CallJoinLeadTime)) {
		return apperrors.Invalid(apperrors.ErrCallNotOpen, "", fmt.Sprintf("the call opens %d minutes before the appointment", int(constant.CallJoinLeadTime.Minutes())))
	}
	if !now.Before(appointment.EndsAt.Add(constant.CallJoinGrace)) {
		return apperrors.Invalid(apperrors.ErrCallNotOpen, "", fmt.Sprintf("the call closed %d minutes after the appointment", int(constant.CallJoinGrace.Minutes())))
	}

	return nil
}

// connected reports whether the participant has an open signaling socket: it
// has not left and was seen within constant.CallPeerTimeout.
func connected(participant *models.CallParticipant, now time.Time) bool {
	return participant.LeftAt == nil && participant.LastSeenAt != nil &&
		now.Sub(*participant.LastSeenAt) < constant.CallPeerTimeout
}

// inRoom reports whether the participant is connected and admitted, so it
// takes part in the media and counts towards the room's size.
func inRoom(participant *models.CallParticipant, now time.Time) bool {
	return participant.AdmissionStatus == models.CallAdmissionAdmitted && connected(participant, now)
}

// appointmentSeatFree reports whether role's seat in an appointment room is
// free for participantID (0 for a new participant). The doctor has one seat
// and the patient side, a guest included, has the other.
func appointmentSeatFree(participants []*models.CallParticipant, participantID uint64, role models.CallParticipantRole, now time.Time) bool {
	doctorSeat := role == models.CallRoleDoctor
	for _, participant := range participants {
		if participant.ID == participantID || !inRoom(participant, now) {
			continue
		}
		if (participant.Role == models.CallRoleDoctor) == doctorSeat {
			return false
		}
	}

	return true
}

// codeRoomMemberFits reports whether one more member can be admitted to a code
// room. The host's seat is kept free, so the host can always join.
func codeRoomMemberFits(room *models.CallRoom, participants []*models.CallParticipant, participantID uint64, now time.Time) bool {
	members := 0
	for _, participant := range participants {
		if participant.ID != participantID && participant.Role != models.CallRoleHost && inRoom(participant, now) {
			members++
		}
	}

	return members < room.MaxParticipants-1
}

// inRoomPeerIDs returns the peer ids of everyone in the room except
// participantID.
func inRoomPeerIDs(participants []*models.CallParticipant, participantID uint64, now time.Time) []string {
	peerIDs := make([]string, 0, len(participants))
	for _, participant := range participants {
		if participant.ID != participantID && inRoom(participant, now) {
			peerIDs = append(peerIDs, participant.PeerID)
		}
	}

	return peerIDs
}

func findCallParticipant(participants []*models.CallParticipant, match func(*models.CallParticipant) bool) *models.CallParticipant {
	for _, participant := range participants {
		if match(participant) {
			return participant
		}
	}

	return nil
}

func callRoomState(room *models.CallRoom, participants []*models.CallParticipant, now time.Time) *models.CallRoomState {
	state := &models.CallRoomState{
		Room:         *room,
		Participants: make([]models.CallPresence, 0, len(participants)),
	}
	for _, participant := range participants {
		state.Participants = append(state.Participants, models.CallPresence{
			Participant: *participant,
			InRoom:      inRoom(participant, now),
		})
	}

	return state
}

func signalPeer(participant *models.CallParticipant) models.CallSignalPeer {
	return models.CallSignalPeer{
		PeerID:      participant.PeerID,
		DisplayName: participant.DisplayName,
		Role:        participant.Role,
		MediaMode:   participant.MediaMode,
		MutedByHost: participant.MutedByHost,
	}
}

func newPeerID() (string, error) {
	peerID, err := utils.NewHexID(constant.CallPeerIDBytes)
	if err != nil {
		return "", fmt.Errorf("generate call peer id: %w", err)
	}

	return peerID, nil
}

// issueCallJoin signs the participant's call token and hands out the ICE
// servers, with TURN credentials named after the peer id.
func issueCallJoin(
	tokens CallTokenIssuer,
	settings ICESettings,
	room *models.CallRoom,
	participant *models.CallParticipant,
	now time.Time,
) (*models.CallJoin, error) {
	token, expiresAt, err := tokens.Generate(models.CallTokenClaims{
		RoomID:        room.ID,
		ParticipantID: participant.ID,
		PeerID:        participant.PeerID,
		Role:          participant.Role,
	})
	if err != nil {
		return nil, fmt.Errorf("generate call token: %w", err)
	}

	servers := make([]models.ICEServer, 0, 2)
	if len(settings.STUNURLs) > 0 {
		servers = append(servers, models.ICEServer{URLs: settings.STUNURLs})
	}
	if len(settings.TURNURLs) > 0 && settings.TURNSecret != "" {
		username, credential := utils.TURNCredential(settings.TURNSecret, participant.PeerID, now.Add(constant.TURNCredentialTTL))
		servers = append(servers, models.ICEServer{URLs: settings.TURNURLs, Username: username, Credential: credential})
	}

	return &models.CallJoin{
		Room:           *room,
		Participant:    *participant,
		Token:          token,
		TokenExpiresAt: expiresAt,
		ICEServers:     servers,
	}, nil
}

// participantFits reports whether participant can be in the room next to the
// others in it.
func participantFits(room *models.CallRoom, participants []*models.CallParticipant, participant *models.CallParticipant, now time.Time) bool {
	if room.Kind == models.CallRoomKindAppointment {
		return appointmentSeatFree(participants, participant.ID, participant.Role, now)
	}

	return participant.Role == models.CallRoleHost || codeRoomMemberFits(room, participants, participant.ID, now)
}

// checkCallJoinAttempts refuses a public join from an IP with
// constant.MaxFailedCallJoinAttempts wrong codes or links within
// constant.CallJoinAttemptWindow.
func checkCallJoinAttempts(ctx context.Context, attempts CallJoinAttemptRepository, clientIP string, now time.Time) error {
	failures, err := attempts.CountSince(ctx, clientIP, now.Add(-constant.CallJoinAttemptWindow))
	if err != nil {
		return fmt.Errorf("count call join attempts: %w", err)
	}
	if failures >= constant.MaxFailedCallJoinAttempts {
		return apperrors.ErrCallJoinAttemptsExceeded
	}

	return nil
}

// recordFailedCallJoin runs outside the join transaction, which has rolled
// back, so the attempt is kept.
func recordFailedCallJoin(ctx context.Context, attempts CallJoinAttemptRepository, clientIP string, now time.Time) error {
	if err := attempts.Create(ctx, clientIP, now); err != nil {
		return fmt.Errorf("record call join attempt: %w", err)
	}
	if err := attempts.DeleteBefore(ctx, clientIP, now.Add(-constant.CallJoinAttemptWindow)); err != nil {
		return fmt.Errorf("delete old call join attempts: %w", err)
	}

	return nil
}

func callEvent(roomID uint64, participantID uint64, eventType models.CallEventType, detail string) *models.CallEvent {
	event := &models.CallEvent{
		CallRoomID: roomID,
		EventType:  eventType,
		Detail:     detail,
	}
	if participantID != 0 {
		event.CallParticipantID = &participantID
	}

	return event
}

// callRoomEvent builds the outbox event for room in its new state.
func callRoomEvent(eventType string, room *models.CallRoom, actingUserID uint64) *models.OutboxEvent {
	payload := map[string]any{
		"callRoomId":      room.ID,
		"kind":            room.Kind,
		"hostUserId":      room.HostUserID,
		"changedByUserId": actingUserID,
		"status":          room.Status,
	}
	if room.AppointmentID != nil {
		payload["appointmentId"] = *room.AppointmentID
	}
	if room.EndReason != "" {
		payload["endReason"] = room.EndReason
	}

	return &models.OutboxEvent{
		AggregateType: models.OutboxAggregateCall,
		AggregateID:   room.ID,
		EventType:     eventType,
		Payload:       payload,
	}
}

// endCallRoom ends a live room with its history row and outbox event. Callers
// run it inside a transaction; it returns ErrCallRoomNotFound when the room
// has already ended.
func endCallRoom(
	txCtx context.Context,
	rooms CallRoomRepository,
	events CallEventRepository,
	outbox OutboxRepository,
	room *models.CallRoom,
	reason models.CallEndReason,
	actingUserID uint64,
	now time.Time,
) error {
	if err := rooms.End(txCtx, room.ID, reason, now); err != nil {
		return err
	}
	if err := events.Create(txCtx, callEvent(room.ID, 0, models.CallEventEnded, string(reason))); err != nil {
		return fmt.Errorf("create call ended event: %w", err)
	}

	ended := *room
	ended.Status = models.CallRoomStatusEnded
	ended.EndReason = reason
	ended.EndedAt = &now

	return outbox.Create(txCtx, callRoomEvent(models.OutboxEventCallEnded, &ended, actingUserID))
}

// endAppointmentCall ends the appointment's live room, if it has one, because
// the appointment was completed, marked no-show or cancelled. Open sockets see
// the room ended on their next heartbeat.
func endAppointmentCall(
	txCtx context.Context,
	rooms CallRoomRepository,
	events CallEventRepository,
	outbox OutboxRepository,
	appointmentID uint64,
	actingUserID uint64,
	now time.Time,
) error {
	room, err := rooms.GetLiveByAppointmentForUpdate(txCtx, appointmentID)
	if err != nil {
		if errors.Is(err, apperrors.ErrCallRoomNotFound) {
			return nil
		}
		return fmt.Errorf("get live call room of appointment: %w", err)
	}

	if err := endCallRoom(txCtx, rooms, events, outbox, room, models.CallEndReasonAppointmentClosed, actingUserID, now); err != nil {
		return fmt.Errorf("end call room of appointment: %w", err)
	}

	return nil
}
