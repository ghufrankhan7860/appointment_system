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
)

// ProposeReschedule lets an appointment's doctor offer a new time. The offered
// time must be a free slot; a placeholder appointment holds it while the
// original keeps its own slot, both in status reschedule_proposed, until the
// patient side accepts or declines, or the offer expires after
// constant.RescheduleProposalTTL or at the original start, whichever is first.
// An empty mode keeps the current one.
func (as *AppointmentService) ProposeReschedule(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	appointmentID uint64,
	startsAt time.Time,
	mode models.ConsultationMode,
	reason models.ProposalReason,
	note string,
) (*models.RescheduleProposal, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("propose reschedule context: %w", err)
	}
	if err := validateDoctorActor(actingUserID, actingRole); err != nil {
		return nil, err
	}
	if appointmentID == 0 {
		return nil, fmt.Errorf("propose reschedule without appointment id: %w", apperrors.ErrInvalidAppointmentInput)
	}
	if !validProposalReason(reason) {
		return nil, apperrors.Invalid(apperrors.ErrInvalidAppointmentInput, "reason", "must be one of emergency, personal, clinic_closed, other")
	}
	note = strings.TrimSpace(note)
	if len(note) > constant.MaxProposalNoteLength {
		return nil, apperrors.Invalid(apperrors.ErrInvalidAppointmentInput, "note", fmt.Sprintf("must be at most %d characters", constant.MaxProposalNoteLength))
	}

	original, link, err := as.accessible(ctx, actingUserID, actingRole, appointmentID)
	if err != nil {
		return nil, err
	}
	if link != nil {
		return nil, apperrors.ErrAppointmentNotFound
	}
	if err := as.checkChangeable(&original.Appointment, nil, "rescheduled"); err != nil {
		return nil, err
	}
	if original.PaymentStatus == models.AppointmentPaymentUnpaid {
		return nil, apperrors.Invalid(apperrors.ErrAppointmentNotModifiable, "", "appointment is awaiting payment; it can be rescheduled once paid")
	}

	if mode == "" {
		mode = original.Mode
	}
	if err := as.validateBooking(&models.Appointment{
		PatientID: original.PatientID,
		DoctorID:  original.DoctorID,
		StartsAt:  startsAt,
		Mode:      mode,
	}); err != nil {
		return nil, err
	}

	doctor, err := as.availability.verifiedDoctor(ctx, original.DoctorID)
	if err != nil {
		return nil, err
	}
	slot, err := as.freeSlot(ctx, doctor, startsAt, mode)
	if err != nil {
		return nil, err
	}

	expiresAt := as.availability.now().Add(constant.RescheduleProposalTTL)
	if original.StartsAt.Before(expiresAt) {
		expiresAt = original.StartsAt
	}

	err = as.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := as.appointments.UpdateStatus(txCtx, original.ID, models.AppointmentStatusScheduled, models.AppointmentStatusRescheduleProposed); err != nil {
			return err
		}
		if err := as.appointments.CreateStatusChange(txCtx, &models.AppointmentStatusChange{
			AppointmentID:   original.ID,
			ChangedByUserID: actingUserID,
			FromStatus:      models.AppointmentStatusScheduled,
			ToStatus:        models.AppointmentStatusRescheduleProposed,
			Reason:          string(reason),
		}); err != nil {
			return err
		}

		placeholder, err := as.appointments.Create(txCtx, &models.Appointment{
			PatientID:         original.PatientID,
			DoctorID:          original.DoctorID,
			BookedByUserID:    original.BookedByUserID,
			StartsAt:          slot.StartsAt,
			EndsAt:            slot.EndsAt,
			Mode:              slot.Mode,
			Status:            models.AppointmentStatusRescheduleProposed,
			ConsultationFee:   original.ConsultationFee,
			SlotDurationMin:   doctor.SlotDurationMin,
			PatientNotes:      original.PatientNotes,
			RescheduledFromID: &original.ID,
			PaymentStatus:     original.PaymentStatus,
		})
		if err != nil {
			return err
		}
		if err := as.appointments.CreateStatusChange(txCtx, &models.AppointmentStatusChange{
			AppointmentID:   placeholder.ID,
			ChangedByUserID: actingUserID,
			ToStatus:        models.AppointmentStatusRescheduleProposed,
		}); err != nil {
			return err
		}

		if _, err := as.appointments.CreateProposal(txCtx, &models.RescheduleProposal{
			AppointmentID:            original.ID,
			PlaceholderAppointmentID: placeholder.ID,
			ProposedByUserID:         actingUserID,
			Reason:                   reason,
			Note:                     note,
			Status:                   models.ProposalStatusPending,
			ExpiresAt:                expiresAt,
		}); err != nil {
			return err
		}

		event := appointmentEvent(models.OutboxEventRescheduleProposed, placeholder, actingUserID)
		event.AggregateID = original.ID
		event.Payload["appointmentId"] = original.ID
		event.Payload["placeholderAppointmentId"] = placeholder.ID
		event.Payload["reason"] = reason
		event.Payload["expiresAt"] = expiresAt
		return as.outbox.Create(txCtx, event)
	})
	if err != nil {
		if errors.Is(err, apperrors.ErrSlotTaken) || errors.Is(err, apperrors.ErrAppointmentNotModifiable) {
			return nil, err
		}
		return nil, fmt.Errorf("propose reschedule of appointment %d: %w", appointmentID, err)
	}

	proposal, err := as.appointments.GetPendingProposal(ctx, original.ID)
	if err != nil {
		return nil, fmt.Errorf("get created reschedule proposal: %w", err)
	}

	as.logger.InfoContext(ctx, "reschedule proposed", "appointment_id", original.ID, "proposal_id", proposal.ID, "placeholder_appointment_id", proposal.PlaceholderAppointmentID, "reason", reason)

	return proposal, nil
}

// GetPendingProposal returns the reschedule proposal waiting on an appointment
// the acting user can see.
func (as *AppointmentService) GetPendingProposal(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	appointmentID uint64,
) (*models.RescheduleProposal, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("get reschedule proposal context: %w", err)
	}
	if actingUserID == 0 || appointmentID == 0 {
		return nil, fmt.Errorf("get reschedule proposal with missing ids: %w", apperrors.ErrInvalidAppointmentInput)
	}

	if _, _, err := as.accessible(ctx, actingUserID, actingRole, appointmentID); err != nil {
		return nil, err
	}

	proposal, err := as.appointments.GetPendingProposal(ctx, appointmentID)
	if err != nil {
		if errors.Is(err, apperrors.ErrProposalNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("get pending reschedule proposal: %w", err)
	}

	return proposal, nil
}

// AcceptReschedule moves the appointment to the doctor's offered time: the
// original is cancelled with reason rescheduled, the placeholder becomes the
// scheduled appointment and the payments move to it. Only the patient side with
// full or appointments_only access can answer.
func (as *AppointmentService) AcceptReschedule(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	appointmentID uint64,
) (*models.AppointmentDetails, error) {
	original, proposal, err := as.pendingProposalToAnswer(ctx, actingUserID, actingRole, appointmentID)
	if err != nil {
		return nil, err
	}

	err = as.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := as.appointments.CloseProposal(txCtx, proposal.ID, models.ProposalStatusAccepted, actingUserID); err != nil {
			return err
		}
		if err := as.appointments.Cancel(txCtx, original.ID, actingUserID, models.CancelReasonRescheduled); err != nil {
			return err
		}
		if err := as.appointments.CreateStatusChange(txCtx, &models.AppointmentStatusChange{
			AppointmentID:   original.ID,
			ChangedByUserID: actingUserID,
			FromStatus:      models.AppointmentStatusRescheduleProposed,
			ToStatus:        models.AppointmentStatusCancelled,
			Reason:          "reschedule proposal accepted",
		}); err != nil {
			return err
		}
		if err := as.appointments.UpdateStatus(txCtx, proposal.PlaceholderAppointmentID, models.AppointmentStatusRescheduleProposed, models.AppointmentStatusScheduled); err != nil {
			return err
		}
		if err := as.appointments.CreateStatusChange(txCtx, &models.AppointmentStatusChange{
			AppointmentID:   proposal.PlaceholderAppointmentID,
			ChangedByUserID: actingUserID,
			FromStatus:      models.AppointmentStatusRescheduleProposed,
			ToStatus:        models.AppointmentStatusScheduled,
		}); err != nil {
			return err
		}
		if err := as.payments.moveToAppointment(txCtx, original.ID, proposal.PlaceholderAppointmentID); err != nil {
			return err
		}

		moved := original.Appointment
		moved.ID = proposal.PlaceholderAppointmentID
		moved.StartsAt = proposal.NewStartsAt
		moved.EndsAt = proposal.NewEndsAt
		moved.Mode = proposal.NewMode
		moved.Status = models.AppointmentStatusScheduled
		moved.RescheduledFromID = &original.ID
		return as.outbox.Create(txCtx, appointmentEvent(models.OutboxEventAppointmentRescheduled, &moved, actingUserID))
	})
	if err != nil {
		if errors.Is(err, apperrors.ErrAppointmentNotModifiable) {
			return nil, apperrors.Invalid(apperrors.ErrAppointmentNotModifiable, "", "the reschedule proposal was answered or expired meanwhile")
		}
		return nil, fmt.Errorf("accept reschedule of appointment %d: %w", appointmentID, err)
	}
	as.logger.InfoContext(ctx, "reschedule accepted", "appointment_id", appointmentID, "proposal_id", proposal.ID, "placeholder_appointment_id", proposal.PlaceholderAppointmentID)

	accepted, err := as.appointments.GetByID(ctx, proposal.PlaceholderAppointmentID)
	if err != nil {
		return nil, fmt.Errorf("get rescheduled appointment: %w", err)
	}

	return accepted, nil
}

// DeclineReschedule keeps the original time: the placeholder is cancelled and
// the original appointment is scheduled again.
func (as *AppointmentService) DeclineReschedule(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	appointmentID uint64,
) (*models.AppointmentDetails, error) {
	original, proposal, err := as.pendingProposalToAnswer(ctx, actingUserID, actingRole, appointmentID)
	if err != nil {
		return nil, err
	}

	if err := as.withdrawProposal(ctx, &original.Appointment, proposal, actingUserID, models.ProposalStatusDeclined); err != nil {
		if errors.Is(err, apperrors.ErrAppointmentNotModifiable) {
			return nil, apperrors.Invalid(apperrors.ErrAppointmentNotModifiable, "", "the reschedule proposal was answered or expired meanwhile")
		}
		return nil, fmt.Errorf("decline reschedule of appointment %d: %w", appointmentID, err)
	}
	as.logger.InfoContext(ctx, "reschedule declined", "appointment_id", appointmentID, "proposal_id", proposal.ID, "placeholder_appointment_id", proposal.PlaceholderAppointmentID)

	declined, err := as.appointments.GetByID(ctx, original.ID)
	if err != nil {
		return nil, fmt.Errorf("get appointment after declining: %w", err)
	}

	return declined, nil
}

// ExpireProposals closes up to limit proposals nobody answered in time, as if
// they were declined, and returns how many it closed. A proposal answered
// meanwhile is skipped.
func (as *AppointmentService) ExpireProposals(ctx context.Context, limit int) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("expire proposals context: %w", err)
	}
	limit, _ = clampPagination(limit, 0)

	proposals, err := as.appointments.ListExpiredProposals(ctx, as.availability.now(), limit)
	if err != nil {
		return 0, fmt.Errorf("list expired reschedule proposals: %w", err)
	}

	expired := 0
	for _, proposal := range proposals {
		original, err := as.appointments.GetByID(ctx, proposal.AppointmentID)
		if err != nil {
			return expired, fmt.Errorf("get appointment of expired proposal %d: %w", proposal.ID, err)
		}

		err = as.withdrawProposal(ctx, &original.Appointment, proposal, 0, models.ProposalStatusExpired)
		if errors.Is(err, apperrors.ErrAppointmentNotModifiable) {
			continue
		}
		if err != nil {
			return expired, fmt.Errorf("expire reschedule proposal %d: %w", proposal.ID, err)
		}
		as.logger.InfoContext(ctx, "reschedule proposal expired", "appointment_id", proposal.AppointmentID, "proposal_id", proposal.ID, "placeholder_appointment_id", proposal.PlaceholderAppointmentID)
		expired++
	}

	return expired, nil
}

// withdrawProposal closes a pending proposal without moving the appointment:
// the placeholder is cancelled and the original is scheduled again.
func (as *AppointmentService) withdrawProposal(
	ctx context.Context,
	original *models.Appointment,
	proposal *models.RescheduleProposal,
	actingUserID uint64,
	toStatus models.ProposalStatus,
) error {
	cancelReason, eventType := models.CancelReasonProposalDeclined, models.OutboxEventRescheduleDeclined
	if toStatus == models.ProposalStatusExpired {
		cancelReason, eventType = models.CancelReasonProposalExpired, models.OutboxEventRescheduleExpired
	}

	return as.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := as.appointments.CloseProposal(txCtx, proposal.ID, toStatus, actingUserID); err != nil {
			return err
		}
		if err := as.appointments.Cancel(txCtx, proposal.PlaceholderAppointmentID, actingUserID, cancelReason); err != nil {
			return err
		}
		if err := as.appointments.CreateStatusChange(txCtx, &models.AppointmentStatusChange{
			AppointmentID:   proposal.PlaceholderAppointmentID,
			ChangedByUserID: actingUserID,
			FromStatus:      models.AppointmentStatusRescheduleProposed,
			ToStatus:        models.AppointmentStatusCancelled,
			Reason:          string(cancelReason),
		}); err != nil {
			return err
		}
		if err := as.appointments.UpdateStatus(txCtx, original.ID, models.AppointmentStatusRescheduleProposed, models.AppointmentStatusScheduled); err != nil {
			return err
		}
		if err := as.appointments.CreateStatusChange(txCtx, &models.AppointmentStatusChange{
			AppointmentID:   original.ID,
			ChangedByUserID: actingUserID,
			FromStatus:      models.AppointmentStatusRescheduleProposed,
			ToStatus:        models.AppointmentStatusScheduled,
			Reason:          string(cancelReason),
		}); err != nil {
			return err
		}

		restored := *original
		restored.Status = models.AppointmentStatusScheduled
		event := appointmentEvent(eventType, &restored, actingUserID)
		event.Payload["placeholderAppointmentId"] = proposal.PlaceholderAppointmentID
		return as.outbox.Create(txCtx, event)
	})
}

// pendingProposalToAnswer loads an appointment whose pending proposal the
// acting user may answer: the patient side with full or appointments_only
// access, before the proposal expires.
func (as *AppointmentService) pendingProposalToAnswer(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	appointmentID uint64,
) (*models.AppointmentDetails, *models.RescheduleProposal, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, fmt.Errorf("answer reschedule proposal context: %w", err)
	}
	if actingUserID == 0 || appointmentID == 0 {
		return nil, nil, fmt.Errorf("answer reschedule proposal with missing ids: %w", apperrors.ErrInvalidAppointmentInput)
	}

	original, link, err := as.accessible(ctx, actingUserID, actingRole, appointmentID)
	if err != nil {
		return nil, nil, err
	}
	if link == nil || !canBook(link) {
		return nil, nil, apperrors.ErrRescheduleForbidden
	}
	if original.Status != models.AppointmentStatusRescheduleProposed {
		return nil, nil, apperrors.ErrProposalNotFound
	}

	proposal, err := as.appointments.GetPendingProposal(ctx, appointmentID)
	if err != nil {
		if errors.Is(err, apperrors.ErrProposalNotFound) {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("get pending reschedule proposal: %w", err)
	}
	if !as.availability.now().Before(proposal.ExpiresAt) {
		return nil, nil, apperrors.Invalid(apperrors.ErrAppointmentNotModifiable, "", "the reschedule proposal has expired")
	}

	return original, proposal, nil
}

func validProposalReason(reason models.ProposalReason) bool {
	switch reason {
	case models.ProposalReasonEmergency, models.ProposalReasonPersonal,
		models.ProposalReasonClinicClosed, models.ProposalReasonOther:
		return true
	default:
		return false
	}
}
