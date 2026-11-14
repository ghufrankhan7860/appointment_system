package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
)

// BookAtDesk books a free slot of the desk doctor (see ClinicActor) for a
// patient at the clinic, on behalf of the patient. The patient must be known
// to the desk: shown their pass there, added as a walk-in, or already in the
// doctor's patient list. The fee is paid at the clinic, so the booking is
// confirmed at once (due_at_clinic, or waived when there is no fee), and it is
// marked booked via the desk. The doctor and slot rules are Book's.
func (as *AppointmentService) BookAtDesk(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	appointment *models.Appointment,
) (*models.AppointmentDetails, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("book at desk context: %w", err)
	}
	if appointment == nil {
		return nil, apperrors.ErrNilAppointmentRequest
	}

	appointment.PatientNotes = strings.TrimSpace(appointment.PatientNotes)
	appointment.IdempotencyKey = strings.TrimSpace(appointment.IdempotencyKey)
	if err := validateIdempotencyKey(appointment.IdempotencyKey); err != nil {
		return nil, err
	}

	deskDoctor, err := as.clinic.DeskDoctor(ctx, actingUserID, actingRole)
	if err != nil {
		return nil, err
	}
	appointment.DoctorID = deskDoctor.ID

	if replayed, err := as.replay(ctx, actingUserID, appointment); replayed != nil || err != nil {
		return replayed, err
	}
	if err := as.validateBooking(appointment); err != nil {
		return nil, err
	}
	if err := as.checkDeskPatient(ctx, deskDoctor.ID, appointment.PatientID); err != nil {
		return nil, err
	}

	doctor, err := as.availability.verifiedDoctor(ctx, deskDoctor.ID)
	if err != nil {
		return nil, err
	}
	slot, err := as.freeSlot(ctx, doctor, appointment.StartsAt, appointment.Mode)
	if err != nil {
		return nil, err
	}

	paymentStatus := models.AppointmentPaymentWaived
	if doctor.ConsultationFee > 0 {
		paymentStatus = models.AppointmentPaymentDueAtClinic
	}

	var created *models.Appointment
	err = as.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		created, err = as.insertBooking(txCtx, actingUserID, &models.Appointment{
			PatientID:       appointment.PatientID,
			DoctorID:        doctor.ID,
			StartsAt:        slot.StartsAt,
			EndsAt:          slot.EndsAt,
			Mode:            slot.Mode,
			ConsultationFee: doctor.ConsultationFee,
			SlotDurationMin: doctor.SlotDurationMin,
			PatientNotes:    appointment.PatientNotes,
			IdempotencyKey:  appointment.IdempotencyKey,
			PaymentStatus:   paymentStatus,
			BookedVia:       models.BookedViaDesk,
		}, models.OutboxEventAppointmentBooked)
		return err
	})
	if err != nil {
		if replayed, replayErr := as.replayAfterClash(ctx, actingUserID, appointment, err); replayed != nil || replayErr != nil {
			return replayed, replayErr
		}
		if errors.Is(err, apperrors.ErrSlotTaken) {
			as.logger.WarnContext(ctx, "slot taken at desk", "doctor_id", doctor.ID, "error", err)
			return nil, err
		}
		return nil, fmt.Errorf("book appointment at desk: %w", err)
	}

	booked, err := as.appointments.GetByID(ctx, created.ID)
	if err != nil {
		return nil, fmt.Errorf("get booked appointment: %w", err)
	}
	as.logger.InfoContext(ctx, "appointment booked at desk",
		"appointment_id", booked.ID,
		"doctor_id", booked.DoctorID,
		"patient_id", booked.PatientID,
		"role", actingRole,
	)

	return booked, nil
}

// checkDeskPatient allows the desk to book for a patient it has seen (pass or
// walk-in) or who is already in the doctor's patient list. Anyone else reads
// as not found, so patient IDs are not revealed.
func (as *AppointmentService) checkDeskPatient(ctx context.Context, doctorID uint64, patientID uint64) error {
	seen, err := as.clinic.HasPatient(ctx, doctorID, patientID)
	if err != nil {
		return err
	}
	if seen {
		return nil
	}

	counted, err := as.appointments.HasCountedBooking(ctx, doctorID, patientID)
	if err != nil {
		return fmt.Errorf("check counted booking: %w", err)
	}
	if !counted {
		return apperrors.ErrPatientNotFound
	}

	return nil
}
