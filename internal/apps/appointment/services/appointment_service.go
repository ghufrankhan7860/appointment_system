// Package services owns appointment business rules, invariants, orchestration, and
// transaction boundaries.
//
// Start a service method from domain behavior, not from an HTTP route or SQL query. Put
// context.Context first, consume narrow repository/provider contracts, return domain
// values plus typed errors, and wrap causes with %w. Constructors must validate required
// dependencies. Test success, invalid state, dependency failure, and cancellation.
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

// AppointmentService books, reschedules, cancels and closes appointments. A
// booking must match a free slot from the availability engine, whose clinic
// timezone and clock it shares. Every status change is written together with a
// history row and an outbox event in one transaction.
type AppointmentService struct {
	appointments AppointmentRepository
	outbox       OutboxRepository
	userPatients UserPatientRepository
	doctors      DoctorRepository
	clinic       *ClinicActor
	availability *AvailabilityService
	payments     *PaymentService
	callRooms    CallRoomRepository
	callEvents   CallEventRepository
	tx           TransactionManager
	logger       Logger
}

func NewAppointmentService(
	appointments AppointmentRepository,
	outbox OutboxRepository,
	userPatients UserPatientRepository,
	doctors DoctorRepository,
	clinic *ClinicActor,
	availability *AvailabilityService,
	payments *PaymentService,
	callRooms CallRoomRepository,
	callEvents CallEventRepository,
	tx TransactionManager,
	logger Logger,
) (*AppointmentService, error) {
	if appointments == nil {
		return nil, apperrors.ErrNilAppointmentRepository
	}
	if outbox == nil {
		return nil, apperrors.ErrNilOutboxRepository
	}
	if userPatients == nil {
		return nil, apperrors.ErrNilUserPatientRepository
	}
	if doctors == nil {
		return nil, apperrors.ErrNilDoctorRepository
	}
	if clinic == nil {
		return nil, apperrors.ErrNilClinicActor
	}
	if availability == nil {
		return nil, apperrors.ErrNilAvailabilityService
	}
	if payments == nil {
		return nil, apperrors.ErrNilPaymentService
	}
	if callRooms == nil {
		return nil, apperrors.ErrNilCallRoomRepository
	}
	if callEvents == nil {
		return nil, apperrors.ErrNilCallEventRepository
	}
	if tx == nil {
		return nil, apperrors.ErrNilTransactionManager
	}
	if logger == nil {
		return nil, apperrors.ErrNilLogger
	}

	return &AppointmentService{
		appointments: appointments,
		outbox:       outbox,
		userPatients: userPatients,
		doctors:      doctors,
		clinic:       clinic,
		availability: availability,
		payments:     payments,
		callRooms:    callRooms,
		callEvents:   callEvents,
		tx:           tx,
		logger:       logger,
	}, nil
}

// Book creates a scheduled appointment for a patient the acting user can book
// for. StartsAt and Mode must match one free slot of the doctor; the end time,
// fee and slot length come from the server, never from the request. With an
// IdempotencyKey, a retry of the same request returns the first booking
// instead of failing because the slot is now taken. A booking with a fee is
// unpaid and holds its slot for constant.PaymentHold; a free one is waived.
func (as *AppointmentService) Book(
	ctx context.Context,
	actingUserID uint64,
	appointment *models.Appointment,
) (*models.AppointmentDetails, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("book appointment context: %w", err)
	}
	if actingUserID == 0 {
		return nil, fmt.Errorf("book appointment without acting user: %w", apperrors.ErrInvalidAppointmentInput)
	}
	if appointment == nil {
		return nil, apperrors.ErrNilAppointmentRequest
	}

	appointment.PatientNotes = strings.TrimSpace(appointment.PatientNotes)
	appointment.IdempotencyKey = strings.TrimSpace(appointment.IdempotencyKey)
	if err := validateIdempotencyKey(appointment.IdempotencyKey); err != nil {
		return nil, err
	}
	if replayed, err := as.replay(ctx, actingUserID, appointment); replayed != nil || err != nil {
		if replayed != nil {
			as.logger.DebugContext(ctx, "booking replayed", "appointment_id", replayed.ID)
		}
		return replayed, err
	}
	if err := as.validateBooking(appointment); err != nil {
		return nil, err
	}

	link, err := as.userPatients.GetActiveByUserAndPatient(ctx, actingUserID, appointment.PatientID)
	if err != nil {
		if errors.Is(err, apperrors.ErrUserPatientNotFound) {
			return nil, apperrors.ErrPatientNotFound
		}
		return nil, fmt.Errorf("authorize appointment booking: %w", err)
	}
	if !canBook(link) {
		return nil, apperrors.ErrPatientForbidden
	}

	doctor, err := as.availability.verifiedDoctor(ctx, appointment.DoctorID)
	if err != nil {
		return nil, err
	}

	slot, err := as.freeSlot(ctx, doctor, appointment.StartsAt, appointment.Mode)
	if err != nil {
		return nil, err
	}

	paymentStatus := models.AppointmentPaymentWaived
	var paymentDueAt *time.Time
	if doctor.ConsultationFee > 0 {
		paymentStatus, paymentDueAt = as.payments.bookingPayment(doctor, as.availability.now())
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
			PaymentDueAt:    paymentDueAt,
		}, models.OutboxEventAppointmentBooked)
		return err
	})
	if err != nil {
		if replayed, replayErr := as.replayAfterClash(ctx, actingUserID, appointment, err); replayed != nil || replayErr != nil {
			return replayed, replayErr
		}
		if errors.Is(err, apperrors.ErrSlotTaken) {
			as.logger.WarnContext(ctx, "slot taken", "doctor_id", doctor.ID, "error", err)
			return nil, err
		}
		return nil, fmt.Errorf("book appointment: %w", err)
	}

	booked, err := as.appointments.GetByID(ctx, created.ID)
	if err != nil {
		return nil, fmt.Errorf("get booked appointment: %w", err)
	}

	as.logger.InfoContext(ctx, "appointment booked",
		"appointment_id", booked.ID,
		"doctor_id", booked.DoctorID,
		"patient_id", booked.PatientID,
		"payment_status", booked.PaymentStatus,
		"payment_due_at", booked.PaymentDueAt,
	)

	return booked, nil
}

// GetByID returns an appointment the acting user may see: one of a patient
// they have an active link to, or one with them as the doctor.
func (as *AppointmentService) GetByID(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	appointmentID uint64,
) (*models.AppointmentDetails, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("get appointment context: %w", err)
	}
	if actingUserID == 0 || appointmentID == 0 {
		return nil, fmt.Errorf("get appointment with missing ids: %w", apperrors.ErrInvalidAppointmentInput)
	}

	appointment, _, err := as.accessible(ctx, actingUserID, actingRole, appointmentID)
	if err != nil {
		return nil, err
	}

	return appointment, nil
}

// Cancel cancels a scheduled appointment that has not started. The patient
// side needs full or appointments_only access and must cancel at least
// constant.AppointmentCancellationWindow before the start; the doctor can
// cancel until the start.
func (as *AppointmentService) Cancel(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	appointmentID uint64,
	reason string,
) (*models.AppointmentDetails, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("cancel appointment context: %w", err)
	}
	if actingUserID == 0 || appointmentID == 0 {
		return nil, fmt.Errorf("cancel appointment with missing ids: %w", apperrors.ErrInvalidAppointmentInput)
	}

	reason = strings.TrimSpace(reason)
	if len(reason) > constant.MaxAppointmentNoteLength {
		return nil, apperrors.Invalid(apperrors.ErrInvalidAppointmentInput, "reason", fmt.Sprintf("must be at most %d characters", constant.MaxAppointmentNoteLength))
	}

	appointment, link, err := as.accessible(ctx, actingUserID, actingRole, appointmentID)
	if err != nil {
		return nil, err
	}

	cancelReason := models.CancelReasonDoctorRequest
	refundReason := models.RefundReasonDoctorCancelled
	if link != nil {
		if !canBook(link) {
			return nil, apperrors.ErrPatientForbidden
		}
		cancelReason = models.CancelReasonPatientRequest
		refundReason = models.RefundReasonPatientCancelled
	}
	if err := as.checkChangeable(&appointment.Appointment, link, "cancelled"); err != nil {
		return nil, err
	}

	err = as.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := as.cancelBooking(txCtx, appointmentID, models.AppointmentStatusScheduled, actingUserID, cancelReason, reason); err != nil {
			return err
		}

		cancelled := appointment.Appointment
		cancelled.Status = models.AppointmentStatusCancelled
		cancelled.CancelReason = cancelReason
		return as.outbox.Create(txCtx, appointmentEvent(models.OutboxEventAppointmentCancelled, &cancelled, actingUserID))
	})
	if err != nil {
		if errors.Is(err, apperrors.ErrAppointmentNotModifiable) {
			return nil, err
		}
		return nil, fmt.Errorf("cancel appointment %d: %w", appointmentID, err)
	}
	as.logger.InfoContext(ctx, "appointment cancelled", "appointment_id", appointmentID, "cancel_reason", cancelReason)
	if err := as.payments.settleCancelled(ctx, appointmentID, actingUserID, refundReason); err != nil {
		return nil, fmt.Errorf("settle payment of cancelled appointment %d: %w", appointmentID, err)
	}

	cancelled, err := as.appointments.GetByID(ctx, appointmentID)
	if err != nil {
		return nil, fmt.Errorf("get cancelled appointment: %w", err)
	}

	return cancelled, nil
}

// Reschedule moves a scheduled appointment to another free slot of the same
// doctor. Only the patient side with full or appointments_only access can do
// it, under the same rules as cancelling. The old appointment is cancelled with
// reason rescheduled and a new one points back at it, keeping the original fee
// and payment: its payment state, due time and payments move to the new one.
// An empty mode keeps the current one.
func (as *AppointmentService) Reschedule(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	appointmentID uint64,
	startsAt time.Time,
	mode models.ConsultationMode,
	idempotencyKey string,
) (*models.AppointmentDetails, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("reschedule appointment context: %w", err)
	}
	if actingUserID == 0 || appointmentID == 0 {
		return nil, fmt.Errorf("reschedule appointment with missing ids: %w", apperrors.ErrInvalidAppointmentInput)
	}

	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if err := validateIdempotencyKey(idempotencyKey); err != nil {
		return nil, err
	}

	current, link, err := as.accessible(ctx, actingUserID, actingRole, appointmentID)
	if err != nil {
		return nil, err
	}
	if link == nil || !canBook(link) {
		return nil, apperrors.ErrRescheduleForbidden
	}

	if mode == "" {
		mode = current.Mode
	}
	request := &models.Appointment{
		PatientID:         current.PatientID,
		DoctorID:          current.DoctorID,
		StartsAt:          startsAt,
		Mode:              mode,
		PatientNotes:      current.PatientNotes,
		RescheduledFromID: &current.ID,
		IdempotencyKey:    idempotencyKey,
	}
	if replayed, err := as.replay(ctx, actingUserID, request); replayed != nil || err != nil {
		return replayed, err
	}
	if err := as.checkChangeable(&current.Appointment, link, "rescheduled"); err != nil {
		return nil, err
	}
	if err := as.validateBooking(request); err != nil {
		return nil, err
	}

	doctor, err := as.availability.verifiedDoctor(ctx, current.DoctorID)
	if err != nil {
		return nil, err
	}
	slot, err := as.freeSlot(ctx, doctor, startsAt, mode)
	if err != nil {
		return nil, err
	}

	var created *models.Appointment
	err = as.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := as.cancelBooking(txCtx, current.ID, models.AppointmentStatusScheduled, actingUserID, models.CancelReasonRescheduled, ""); err != nil {
			return err
		}

		// A direct UPI payment waiting for its check is due by the new start.
		paymentDueAt := current.PaymentDueAt
		if current.PaymentStatus == models.AppointmentPaymentPendingVerification {
			startsAt := slot.StartsAt
			paymentDueAt = &startsAt
		}
		created, err = as.insertBooking(txCtx, actingUserID, &models.Appointment{
			PatientID:         current.PatientID,
			DoctorID:          current.DoctorID,
			StartsAt:          slot.StartsAt,
			EndsAt:            slot.EndsAt,
			Mode:              slot.Mode,
			ConsultationFee:   current.ConsultationFee,
			SlotDurationMin:   doctor.SlotDurationMin,
			PatientNotes:      current.PatientNotes,
			RescheduledFromID: &current.ID,
			IdempotencyKey:    idempotencyKey,
			PaymentStatus:     current.PaymentStatus,
			PaymentDueAt:      paymentDueAt,
			BookedVia:         current.BookedVia,
		}, models.OutboxEventAppointmentRescheduled)
		if err != nil {
			return err
		}

		return as.payments.moveToAppointment(txCtx, current.ID, created.ID)
	})
	if err != nil {
		if replayed, replayErr := as.replayAfterClash(ctx, actingUserID, request, err); replayed != nil || replayErr != nil {
			return replayed, replayErr
		}
		if errors.Is(err, apperrors.ErrSlotTaken) || errors.Is(err, apperrors.ErrAppointmentNotModifiable) {
			as.logger.WarnContext(ctx, "reschedule refused", "appointment_id", appointmentID, "error", err)
			return nil, err
		}
		return nil, fmt.Errorf("reschedule appointment %d: %w", appointmentID, err)
	}
	as.logger.InfoContext(ctx, "appointment rescheduled", "from_appointment_id", appointmentID, "appointment_id", created.ID)

	rescheduled, err := as.appointments.GetByID(ctx, created.ID)
	if err != nil {
		return nil, fmt.Errorf("get rescheduled appointment: %w", err)
	}

	return rescheduled, nil
}

// Complete marks a scheduled appointment that has started as completed. Only
// the appointment's doctor can do it, and only once it is paid or free.
func (as *AppointmentService) Complete(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	appointmentID uint64,
) (*models.AppointmentDetails, error) {
	return as.close(ctx, actingUserID, actingRole, appointmentID, models.AppointmentStatusCompleted, models.OutboxEventAppointmentCompleted)
}

// MarkNoShow marks a scheduled appointment that has started as a no-show. Only
// the appointment's doctor can do it.
func (as *AppointmentService) MarkNoShow(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	appointmentID uint64,
) (*models.AppointmentDetails, error) {
	return as.close(ctx, actingUserID, actingRole, appointmentID, models.AppointmentStatusNoShow, models.OutboxEventAppointmentNoShow)
}

// List returns the appointments of every patient the acting user has an active
// link to, earliest first.
func (as *AppointmentService) List(
	ctx context.Context,
	actingUserID uint64,
	filter models.AppointmentFilter,
) ([]*models.AppointmentDetails, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list appointments context: %w", err)
	}
	if actingUserID == 0 {
		return nil, fmt.Errorf("list appointments without acting user: %w", apperrors.ErrInvalidAppointmentInput)
	}
	if filter.Status != "" && !validAppointmentStatus(filter.Status) {
		return nil, apperrors.Invalid(apperrors.ErrInvalidAppointmentInput, "status", "must be one of scheduled, completed, cancelled, no_show, reschedule_proposed")
	}
	if !filter.From.IsZero() && !filter.To.IsZero() && !filter.To.After(filter.From) {
		return nil, apperrors.Invalid(apperrors.ErrInvalidAppointmentInput, "to", "must be after from")
	}

	filter.Limit, filter.Offset = clampPagination(filter.Limit, filter.Offset)

	appointments, err := as.appointments.ListByUser(ctx, actingUserID, filter)
	if err != nil {
		return nil, fmt.Errorf("list appointments by user: %w", err)
	}

	return appointments, nil
}

// ListForDoctor returns the appointments in any status on one clinic date,
// earliest first, of the doctor whose desk the acting user runs: their own for
// a doctor, their doctor's for staff.
func (as *AppointmentService) ListForDoctor(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	date time.Time,
) ([]*models.AppointmentDetails, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list doctor appointments context: %w", err)
	}
	doctor, err := as.clinic.DeskDoctor(ctx, actingUserID, actingRole)
	if err != nil {
		return nil, err
	}

	date = utils.CivilDate(date)
	dayStart := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, as.availability.location)
	appointments, err := as.appointments.ListByDoctorBetween(ctx, doctor.ID, dayStart, dayStart.AddDate(0, 0, 1))
	if err != nil {
		return nil, fmt.Errorf("list appointments by doctor: %w", err)
	}

	return appointments, nil
}

// ListPatients returns the desk doctor's patients (see ListForDoctor): everyone with a booking
// that counts (completed, no-show, or scheduled and not an unpaid hold), the
// soonest next visit first. A non-empty query matches the patient's name.
func (as *AppointmentService) ListPatients(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	query string,
	limit int,
	offset int,
) ([]*models.DoctorPatient, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list doctor patients context: %w", err)
	}
	doctor, err := as.clinic.DeskDoctor(ctx, actingUserID, actingRole)
	if err != nil {
		return nil, err
	}

	limit, offset = clampPagination(limit, offset)

	patients, err := as.appointments.ListPatientsByDoctor(ctx, doctor.ID, strings.TrimSpace(query), as.availability.now(), limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list patients by doctor: %w", err)
	}

	return patients, nil
}

// Location is the clinic timezone that appointment dates and clock times are local to.
func (as *AppointmentService) Location() *time.Location {
	return as.availability.location
}

func (as *AppointmentService) validateBooking(appointment *models.Appointment) error {
	if appointment.PatientID == 0 {
		return apperrors.Invalid(apperrors.ErrInvalidAppointmentInput, "patientId", "is required")
	}
	if appointment.DoctorID == 0 {
		return apperrors.Invalid(apperrors.ErrInvalidAppointmentInput, "doctorId", "is required")
	}
	if appointment.StartsAt.IsZero() {
		return apperrors.Invalid(apperrors.ErrInvalidAppointmentInput, "startsAt", "is required")
	}
	if !validConsultationMode(appointment.Mode) {
		return apperrors.Invalid(apperrors.ErrInvalidAppointmentInput, "mode", "must be one of in_person, video, phone")
	}
	if len(appointment.PatientNotes) > constant.MaxAppointmentNoteLength {
		return apperrors.Invalid(apperrors.ErrInvalidAppointmentInput, "notes", fmt.Sprintf("must be at most %d characters", constant.MaxAppointmentNoteLength))
	}

	if !appointment.StartsAt.After(as.availability.now()) {
		return apperrors.Invalid(apperrors.ErrInvalidAppointmentInput, "startsAt", "must be in the future")
	}
	date := utils.CivilDate(appointment.StartsAt.In(as.availability.location))
	if date.After(as.availability.today().AddDate(0, 0, constant.SlotHorizonDays)) {
		return apperrors.Invalid(apperrors.ErrInvalidAppointmentInput, "startsAt", fmt.Sprintf("must be within %d days", constant.SlotHorizonDays))
	}

	return nil
}

func (as *AppointmentService) close(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	appointmentID uint64,
	toStatus models.AppointmentStatus,
	eventType string,
) (*models.AppointmentDetails, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("close appointment context: %w", err)
	}
	if err := validateDoctorActor(actingUserID, actingRole); err != nil {
		return nil, err
	}
	if appointmentID == 0 {
		return nil, fmt.Errorf("close appointment without id: %w", apperrors.ErrInvalidAppointmentInput)
	}

	appointment, link, err := as.accessible(ctx, actingUserID, actingRole, appointmentID)
	if err != nil {
		return nil, err
	}
	if link != nil {
		return nil, apperrors.ErrAppointmentNotFound
	}
	if appointment.Status == models.AppointmentStatusRescheduleProposed {
		return nil, apperrors.Invalid(apperrors.ErrAppointmentNotModifiable, "", "a reschedule proposal is waiting for the patient's answer")
	}
	if appointment.Status != models.AppointmentStatusScheduled {
		return nil, apperrors.Invalid(apperrors.ErrAppointmentNotModifiable, "", fmt.Sprintf("appointment is %s; only a scheduled appointment can be marked %s", appointment.Status, toStatus))
	}
	if as.availability.now().Before(appointment.StartsAt) {
		return nil, apperrors.Invalid(apperrors.ErrAppointmentNotModifiable, "", "appointment has not started yet")
	}
	if toStatus == models.AppointmentStatusCompleted &&
		appointment.PaymentStatus != models.AppointmentPaymentPaid && appointment.PaymentStatus != models.AppointmentPaymentWaived &&
		appointment.PaymentStatus != models.AppointmentPaymentDueAtClinic {
		return nil, apperrors.Invalid(apperrors.ErrPaymentPending, "", fmt.Sprintf("payment is %s for this appointment", appointment.PaymentStatus))
	}

	err = as.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := as.appointments.UpdateStatus(txCtx, appointmentID, models.AppointmentStatusScheduled, toStatus); err != nil {
			return err
		}
		if err := as.appointments.CreateStatusChange(txCtx, &models.AppointmentStatusChange{
			AppointmentID:   appointmentID,
			ChangedByUserID: actingUserID,
			FromStatus:      models.AppointmentStatusScheduled,
			ToStatus:        toStatus,
		}); err != nil {
			return err
		}
		if err := as.endCall(txCtx, appointmentID, actingUserID); err != nil {
			return err
		}

		closed := appointment.Appointment
		closed.Status = toStatus
		return as.outbox.Create(txCtx, appointmentEvent(eventType, &closed, actingUserID))
	})
	if err != nil {
		if errors.Is(err, apperrors.ErrAppointmentNotModifiable) {
			return nil, err
		}
		return nil, fmt.Errorf("mark appointment %d %s: %w", appointmentID, toStatus, err)
	}
	as.logger.InfoContext(ctx, "appointment closed", "appointment_id", appointmentID, "status", toStatus)

	closed, err := as.appointments.GetByID(ctx, appointmentID)
	if err != nil {
		return nil, fmt.Errorf("get closed appointment: %w", err)
	}

	return closed, nil
}

// checkChangeable applies the rules shared by cancel and reschedule: only a
// scheduled appointment that has not started, and the patient side (a non-nil
// link) no later than the cancellation window before the start.
func (as *AppointmentService) checkChangeable(appointment *models.Appointment, link *models.UserPatient, action string) error {
	if appointment.Status == models.AppointmentStatusRescheduleProposed {
		return apperrors.Invalid(apperrors.ErrAppointmentNotModifiable, "", "the doctor proposed a new time; accept or decline it first")
	}
	if appointment.Status != models.AppointmentStatusScheduled {
		return apperrors.Invalid(apperrors.ErrAppointmentNotModifiable, "", fmt.Sprintf("appointment is %s; only a scheduled appointment can be %s", appointment.Status, action))
	}
	now := as.availability.now()
	if !now.Before(appointment.StartsAt) {
		return apperrors.Invalid(apperrors.ErrAppointmentNotModifiable, "", "appointment has already started")
	}
	if link != nil && now.After(appointment.StartsAt.Add(-constant.AppointmentCancellationWindow)) {
		return apperrors.Invalid(apperrors.ErrCancellationWindowPassed, "", fmt.Sprintf("appointments can be %s up to %d hours before they start", action, int(constant.AppointmentCancellationWindow/time.Hour)))
	}

	return nil
}

func (as *AppointmentService) freeSlot(
	ctx context.Context,
	doctor *models.Doctor,
	startsAt time.Time,
	mode models.ConsultationMode,
) (*models.Slot, error) {
	date := utils.CivilDate(startsAt.In(as.availability.location))
	slots, err := as.availability.freeSlots(ctx, doctor, date)
	if err != nil {
		return nil, err
	}

	for index := range slots {
		if slots[index].StartsAt.Equal(startsAt) && slots[index].Mode == mode {
			return &slots[index], nil
		}
	}

	return nil, apperrors.Invalid(apperrors.ErrSlotNotAvailable, "startsAt", "is not a free slot for this doctor and mode")
}

// insertBooking writes a scheduled appointment, its first history row and its
// outbox event. Callers run it inside a transaction.
func (as *AppointmentService) insertBooking(
	txCtx context.Context,
	actingUserID uint64,
	appointment *models.Appointment,
	eventType string,
) (*models.Appointment, error) {
	appointment.BookedByUserID = actingUserID
	appointment.Status = models.AppointmentStatusScheduled

	created, err := as.appointments.Create(txCtx, appointment)
	if err != nil {
		return nil, err
	}
	if err := as.appointments.CreateStatusChange(txCtx, &models.AppointmentStatusChange{
		AppointmentID:   created.ID,
		ChangedByUserID: actingUserID,
		ToStatus:        models.AppointmentStatusScheduled,
	}); err != nil {
		return nil, err
	}
	if err := as.outbox.Create(txCtx, appointmentEvent(eventType, created, actingUserID)); err != nil {
		return nil, err
	}

	return created, nil
}

// cancelBooking cancels a scheduled or reschedule_proposed appointment and
// writes its history row. Callers run it inside a transaction.
func (as *AppointmentService) cancelBooking(
	txCtx context.Context,
	appointmentID uint64,
	fromStatus models.AppointmentStatus,
	actingUserID uint64,
	cancelReason models.CancelReason,
	note string,
) error {
	if err := as.appointments.Cancel(txCtx, appointmentID, actingUserID, cancelReason); err != nil {
		return err
	}
	if err := as.appointments.CreateStatusChange(txCtx, &models.AppointmentStatusChange{
		AppointmentID:   appointmentID,
		ChangedByUserID: actingUserID,
		FromStatus:      fromStatus,
		ToStatus:        models.AppointmentStatusCancelled,
		Reason:          note,
	}); err != nil {
		return err
	}

	return as.endCall(txCtx, appointmentID, actingUserID)
}

// endCall ends the appointment's live call, if any, inside the caller's
// transaction that closes or cancels the appointment.
func (as *AppointmentService) endCall(txCtx context.Context, appointmentID uint64, actingUserID uint64) error {
	return endAppointmentCall(txCtx, as.callRooms, as.callEvents, as.outbox, appointmentID, actingUserID, as.availability.now())
}

// replay returns the appointment the acting user already created with
// request's IdempotencyKey, or nil when the key is empty or unused. A key used
// for a different request is ErrIdempotencyKeyReused.
func (as *AppointmentService) replay(
	ctx context.Context,
	actingUserID uint64,
	request *models.Appointment,
) (*models.AppointmentDetails, error) {
	if request.IdempotencyKey == "" {
		return nil, nil
	}

	existing, err := as.appointments.GetByIdempotencyKey(ctx, actingUserID, request.IdempotencyKey)
	if err != nil {
		if errors.Is(err, apperrors.ErrAppointmentNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("get appointment by idempotency key: %w", err)
	}

	sameSource := (existing.RescheduledFromID == nil) == (request.RescheduledFromID == nil) &&
		(existing.RescheduledFromID == nil || *existing.RescheduledFromID == *request.RescheduledFromID)
	if !sameSource || existing.PatientID != request.PatientID || existing.DoctorID != request.DoctorID ||
		!existing.StartsAt.Equal(request.StartsAt) || existing.Mode != request.Mode {
		return nil, apperrors.Invalid(apperrors.ErrIdempotencyKeyReused, "", "Idempotency-Key was already used for a different request")
	}

	return existing, nil
}

// replayAfterClash handles two concurrent requests with the same key: the one
// that loses the insert returns what the winner created.
func (as *AppointmentService) replayAfterClash(
	ctx context.Context,
	actingUserID uint64,
	request *models.Appointment,
	err error,
) (*models.AppointmentDetails, error) {
	if !errors.Is(err, apperrors.ErrIdempotencyKeyExists) && !errors.Is(err, apperrors.ErrSlotTaken) &&
		!errors.Is(err, apperrors.ErrAppointmentNotModifiable) {
		return nil, nil
	}

	return as.replay(ctx, actingUserID, request)
}

// accessible loads an appointment and checks that the acting user may see it.
// The link is nil when the acting user is the appointment's doctor or that
// doctor's staff (the clinic desk). Anyone else gets ErrAppointmentNotFound,
// so appointment IDs are not revealed.
func (as *AppointmentService) accessible(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	appointmentID uint64,
) (*models.AppointmentDetails, *models.UserPatient, error) {
	appointment, err := as.appointments.GetByID(ctx, appointmentID)
	if err != nil {
		if errors.Is(err, apperrors.ErrAppointmentNotFound) {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("get appointment by id: %w", err)
	}

	desk, err := as.clinic.IsDeskOf(ctx, actingUserID, actingRole, appointment.DoctorID)
	if err != nil {
		return nil, nil, fmt.Errorf("check clinic desk: %w", err)
	}
	if desk {
		return appointment, nil, nil
	}

	link, err := as.userPatients.GetActiveByUserAndPatient(ctx, actingUserID, appointment.PatientID)
	if err != nil {
		if errors.Is(err, apperrors.ErrUserPatientNotFound) {
			return nil, nil, apperrors.ErrAppointmentNotFound
		}
		return nil, nil, fmt.Errorf("authorize appointment access: %w", err)
	}

	return appointment, link, nil
}

func canBook(link *models.UserPatient) bool {
	return link.AccessLevel == models.AccessLevelFull || link.AccessLevel == models.AccessLevelAppointmentsOnly
}

func validateIdempotencyKey(key string) error {
	if len(key) > constant.MaxIdempotencyKeyLength {
		return apperrors.Invalid(apperrors.ErrInvalidAppointmentInput, "", fmt.Sprintf("Idempotency-Key must be at most %d characters", constant.MaxIdempotencyKeyLength))
	}

	return nil
}

// appointmentEvent builds the outbox event for appointment in its new state.
func appointmentEvent(eventType string, appointment *models.Appointment, actingUserID uint64) *models.OutboxEvent {
	payload := map[string]any{
		"appointmentId":   appointment.ID,
		"patientId":       appointment.PatientID,
		"doctorId":        appointment.DoctorID,
		"bookedByUserId":  appointment.BookedByUserID,
		"changedByUserId": actingUserID,
		"startsAt":        appointment.StartsAt,
		"endsAt":          appointment.EndsAt,
		"mode":            appointment.Mode,
		"status":          appointment.Status,
		"paymentStatus":   appointment.PaymentStatus,
	}
	if appointment.CancelReason != "" {
		payload["cancelReason"] = appointment.CancelReason
	}
	if appointment.RescheduledFromID != nil {
		payload["rescheduledFromId"] = *appointment.RescheduledFromID
	}

	return &models.OutboxEvent{
		AggregateType: models.OutboxAggregateAppointment,
		AggregateID:   appointment.ID,
		EventType:     eventType,
		Payload:       payload,
	}
}

func validAppointmentStatus(status models.AppointmentStatus) bool {
	switch status {
	case models.AppointmentStatusScheduled, models.AppointmentStatusCompleted,
		models.AppointmentStatusCancelled, models.AppointmentStatusNoShow,
		models.AppointmentStatusRescheduleProposed:
		return true
	default:
		return false
	}
}
