package services

import (
	"context"
	"errors"
	"fmt"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
)

// cancelForSuspension cancels every booking of a suspended doctor that has not
// started yet, with reason doctor_suspended. It runs inside the suspension's
// transaction, so the doctor and the bookings change together. A booking with
// a pending reschedule proposal is cancelled with the proposal, closed as
// cancelled, and its placeholder; a placeholder is never handled on its own.
// The cancelled bookings are returned so their payments can be settled after
// the commit with settleSuspended.
func (as *AppointmentService) cancelForSuspension(
	txCtx context.Context,
	doctorID uint64,
	actingUserID uint64,
) ([]*models.AppointmentDetails, error) {
	now := as.availability.now()
	// Bookings are never further ahead than the slot horizon.
	upcoming, err := as.appointments.ListByDoctorBetween(txCtx, doctorID, now, now.AddDate(0, 0, constant.SlotHorizonDays+1))
	if err != nil {
		return nil, fmt.Errorf("list upcoming appointments of doctor %d: %w", doctorID, err)
	}

	var cancelled []*models.AppointmentDetails
	for _, appointment := range upcoming {
		if !appointment.StartsAt.After(now) {
			continue
		}

		switch appointment.Status {
		case models.AppointmentStatusScheduled:
		case models.AppointmentStatusRescheduleProposed:
			proposal, err := as.appointments.GetPendingProposal(txCtx, appointment.ID)
			if errors.Is(err, apperrors.ErrProposalNotFound) {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("get pending proposal of appointment %d: %w", appointment.ID, err)
			}
			if err := as.appointments.CloseProposal(txCtx, proposal.ID, models.ProposalStatusCancelled, actingUserID); err != nil {
				return nil, fmt.Errorf("cancel proposal %d: %w", proposal.ID, err)
			}
			if err := as.cancelBooking(txCtx, proposal.PlaceholderAppointmentID, models.AppointmentStatusRescheduleProposed, actingUserID, models.CancelReasonDoctorSuspended, ""); err != nil {
				return nil, fmt.Errorf("cancel placeholder %d: %w", proposal.PlaceholderAppointmentID, err)
			}
		default:
			continue
		}

		if err := as.cancelBooking(txCtx, appointment.ID, appointment.Status, actingUserID, models.CancelReasonDoctorSuspended, ""); err != nil {
			return nil, fmt.Errorf("cancel appointment %d: %w", appointment.ID, err)
		}
		event := appointment.Appointment
		event.Status = models.AppointmentStatusCancelled
		event.CancelReason = models.CancelReasonDoctorSuspended
		if err := as.outbox.Create(txCtx, appointmentEvent(models.OutboxEventAppointmentCancelled, &event, actingUserID)); err != nil {
			return nil, fmt.Errorf("write cancel event of appointment %d: %w", appointment.ID, err)
		}

		cancelled = append(cancelled, appointment)
	}

	return cancelled, nil
}

// settleSuspended settles the payments of bookings cancelled by a suspension:
// paid ones are refunded in full as doctor_cancelled and open orders expire.
// It stops at the first failure; the bookings stay cancelled and what is left
// can be refunded with PaymentService.Refund.
func (as *AppointmentService) settleSuspended(
	ctx context.Context,
	cancelled []*models.AppointmentDetails,
	actingUserID uint64,
) error {
	for _, appointment := range cancelled {
		if err := as.payments.settleCancelled(ctx, appointment.ID, actingUserID, models.RefundReasonDoctorCancelled); err != nil {
			return fmt.Errorf("settle payment of appointment %d: %w", appointment.ID, err)
		}
	}

	return nil
}
