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

// PaymentService collects appointment fees through the configured gateway and
// refunds them. Payment comes before confirmation: a booking with a fee is
// unpaid and holds its slot until PaymentDueAt, and ExpireUnpaid cancels the
// ones never paid. Gateway calls never run inside a database transaction.
type PaymentService struct {
	payments     PaymentRepository
	appointments AppointmentRepository
	outbox       OutboxRepository
	userPatients UserPatientRepository
	doctors      DoctorRepository
	gateway      PaymentGateway
	tx           TransactionManager
	now          func() time.Time
}

func NewPaymentService(
	payments PaymentRepository,
	appointments AppointmentRepository,
	outbox OutboxRepository,
	userPatients UserPatientRepository,
	doctors DoctorRepository,
	gateway PaymentGateway,
	tx TransactionManager,
	now func() time.Time,
) (*PaymentService, error) {
	if payments == nil {
		return nil, apperrors.ErrNilPaymentRepository
	}
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
	if gateway == nil {
		return nil, apperrors.ErrNilPaymentGateway
	}
	if tx == nil {
		return nil, apperrors.ErrNilTransactionManager
	}
	if now == nil {
		return nil, apperrors.ErrNilClock
	}

	return &PaymentService{
		payments:     payments,
		appointments: appointments,
		outbox:       outbox,
		userPatients: userPatients,
		doctors:      doctors,
		gateway:      gateway,
		tx:           tx,
		now:          now,
	}, nil
}

// Create opens a gateway order for an unpaid appointment and returns the
// payment with what the client needs to pay it. Anyone with full or
// appointments_only access can pay, not only the booker. The amount is the fee
// stored on the appointment. Asking again returns the same live payment.
func (ps *PaymentService) Create(
	ctx context.Context,
	actingUserID uint64,
	appointmentID uint64,
) (*models.Payment, map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, fmt.Errorf("create payment context: %w", err)
	}
	if actingUserID == 0 || appointmentID == 0 {
		return nil, nil, fmt.Errorf("create payment with missing ids: %w", apperrors.ErrInvalidPaymentInput)
	}

	appointment, err := ps.payableAppointment(ctx, actingUserID, appointmentID)
	if err != nil {
		return nil, nil, err
	}
	if err := ps.checkPayable(appointment); err != nil {
		return nil, nil, err
	}

	if existing, err := ps.payments.GetActiveByAppointment(ctx, appointmentID); err == nil {
		return existing, ps.gateway.ClientPayload(existing), nil
	} else if !errors.Is(err, apperrors.ErrPaymentNotFound) {
		return nil, nil, fmt.Errorf("get active payment: %w", err)
	}

	orderID, err := ps.gateway.CreateOrder(ctx, appointmentID, appointment.ConsultationFee, constant.PaymentCurrency)
	if err != nil {
		return nil, nil, fmt.Errorf("create gateway order: %w", err)
	}

	var created *models.Payment
	err = ps.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		created, err = ps.payments.Create(txCtx, &models.Payment{
			AppointmentID:   appointmentID,
			PayerUserID:     actingUserID,
			Amount:          appointment.ConsultationFee,
			Currency:        constant.PaymentCurrency,
			Provider:        ps.gateway.Provider(),
			ProviderOrderID: orderID,
			Status:          models.PaymentStatusCreated,
		})
		if err != nil {
			return err
		}

		return ps.payments.CreateStatusChange(txCtx, &models.PaymentStatusChange{
			PaymentID:       created.ID,
			ChangedByUserID: actingUserID,
			ToStatus:        models.PaymentStatusCreated,
		})
	})
	if err != nil {
		if errors.Is(err, apperrors.ErrPaymentAlreadyExists) {
			existing, getErr := ps.payments.GetActiveByAppointment(ctx, appointmentID)
			if getErr != nil {
				return nil, nil, fmt.Errorf("get payment created meanwhile: %w", getErr)
			}
			return existing, ps.gateway.ClientPayload(existing), nil
		}
		return nil, nil, fmt.Errorf("create payment: %w", err)
	}

	return created, ps.gateway.ClientPayload(created), nil
}

// Verify confirms with the gateway that a created payment was paid, captures
// it and marks the appointment paid in one transaction. Verifying a captured
// payment again with the same payment id returns it unchanged. When the booking
// expired first, the money is refunded and ErrPaymentWindowClosed is returned.
func (ps *PaymentService) Verify(
	ctx context.Context,
	actingUserID uint64,
	paymentID uint64,
	providerPaymentID string,
	signature string,
) (*models.Payment, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("verify payment context: %w", err)
	}
	if actingUserID == 0 || paymentID == 0 {
		return nil, fmt.Errorf("verify payment with missing ids: %w", apperrors.ErrInvalidPaymentInput)
	}
	providerPaymentID = strings.TrimSpace(providerPaymentID)
	if providerPaymentID == "" {
		return nil, apperrors.Invalid(apperrors.ErrInvalidPaymentInput, "providerPaymentId", "is required")
	}
	if strings.TrimSpace(signature) == "" {
		return nil, apperrors.Invalid(apperrors.ErrInvalidPaymentInput, "signature", "is required")
	}

	payment, err := ps.payments.GetByID(ctx, paymentID)
	if err != nil {
		if errors.Is(err, apperrors.ErrPaymentNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("get payment by id: %w", err)
	}
	appointment, err := ps.payableAppointment(ctx, actingUserID, payment.AppointmentID)
	if err != nil {
		if errors.Is(err, apperrors.ErrAppointmentNotFound) {
			return nil, apperrors.ErrPaymentNotFound
		}
		return nil, err
	}

	if payment.Status == models.PaymentStatusCaptured && payment.ProviderPaymentID == providerPaymentID {
		return payment, nil
	}
	if payment.Status != models.PaymentStatusCreated {
		return nil, apperrors.Invalid(apperrors.ErrPaymentNotModifiable, "", fmt.Sprintf("payment is %s", payment.Status))
	}
	if err := ps.gateway.VerifyPayment(ctx, payment, providerPaymentID, signature); err != nil {
		if errors.Is(err, apperrors.ErrPaymentVerificationFailed) {
			return nil, err
		}
		return nil, fmt.Errorf("verify payment with gateway: %w", err)
	}

	now := ps.now()
	late := appointment.Status != models.AppointmentStatusScheduled ||
		appointment.PaymentStatus != models.AppointmentPaymentUnpaid ||
		appointment.PaymentDueAt == nil || !now.Before(*appointment.PaymentDueAt)

	err = ps.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := ps.payments.UpdateStatus(txCtx, paymentID, models.PaymentStatusCreated, models.PaymentStatusCaptured, providerPaymentID, &now); err != nil {
			return err
		}
		if err := ps.payments.CreateStatusChange(txCtx, &models.PaymentStatusChange{
			PaymentID:       paymentID,
			ChangedByUserID: actingUserID,
			FromStatus:      models.PaymentStatusCreated,
			ToStatus:        models.PaymentStatusCaptured,
		}); err != nil {
			return err
		}
		if late {
			return nil
		}

		if err := ps.appointments.MarkPaid(txCtx, appointment.ID); err != nil {
			if errors.Is(err, apperrors.ErrAppointmentNotModifiable) {
				late = true
				return nil
			}
			return err
		}

		payment.Status = models.PaymentStatusCaptured
		return ps.outbox.Create(txCtx, paymentEvent(models.OutboxEventPaymentCaptured, payment))
	})
	if err != nil {
		if errors.Is(err, apperrors.ErrProviderPaymentUsed) {
			return nil, apperrors.Invalid(apperrors.ErrPaymentVerificationFailed, "providerPaymentId", "was already used for another payment")
		}
		if errors.Is(err, apperrors.ErrPaymentNotModifiable) {
			return nil, err
		}
		return nil, fmt.Errorf("capture payment %d: %w", paymentID, err)
	}

	captured, err := ps.payments.GetByID(ctx, paymentID)
	if err != nil {
		return nil, fmt.Errorf("get captured payment: %w", err)
	}
	if late {
		if err := ps.refund(ctx, captured, models.RefundReasonLatePayment, 0, ""); err != nil {
			return nil, fmt.Errorf("refund late payment %d: %w", paymentID, err)
		}
		return nil, apperrors.Invalid(apperrors.ErrPaymentWindowClosed, "", "the booking expired before the payment arrived; the payment has been refunded")
	}

	return captured, nil
}

// GetForAppointment returns the appointment's most recent payment. The patient
// side with any active link, the appointment's doctor and admins can see it.
func (ps *PaymentService) GetForAppointment(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	appointmentID uint64,
) (*models.Payment, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("get appointment payment context: %w", err)
	}
	if actingUserID == 0 || appointmentID == 0 {
		return nil, fmt.Errorf("get appointment payment with missing ids: %w", apperrors.ErrInvalidPaymentInput)
	}

	appointment, err := ps.appointments.GetByID(ctx, appointmentID)
	if err != nil {
		if errors.Is(err, apperrors.ErrAppointmentNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("get appointment by id: %w", err)
	}
	if err := ps.checkCanSee(ctx, actingUserID, actingRole, appointment); err != nil {
		return nil, err
	}

	payment, err := ps.payments.GetLatestByAppointment(ctx, appointmentID)
	if err != nil {
		if errors.Is(err, apperrors.ErrPaymentNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("get latest payment: %w", err)
	}

	return payment, nil
}

// Refund returns a captured payment in full on an admin's request. The route
// permission decides who is an admin.
func (ps *PaymentService) Refund(
	ctx context.Context,
	actingUserID uint64,
	paymentID uint64,
	note string,
) (*models.Payment, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("refund payment context: %w", err)
	}
	if actingUserID == 0 || paymentID == 0 {
		return nil, fmt.Errorf("refund payment with missing ids: %w", apperrors.ErrInvalidPaymentInput)
	}
	note = strings.TrimSpace(note)
	if len(note) > constant.MaxRefundNoteLength {
		return nil, apperrors.Invalid(apperrors.ErrInvalidPaymentInput, "note", fmt.Sprintf("must be at most %d characters", constant.MaxRefundNoteLength))
	}

	payment, err := ps.payments.GetByID(ctx, paymentID)
	if err != nil {
		if errors.Is(err, apperrors.ErrPaymentNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("get payment by id: %w", err)
	}
	if payment.Status != models.PaymentStatusCaptured {
		return nil, apperrors.Invalid(apperrors.ErrPaymentNotModifiable, "", fmt.Sprintf("payment is %s; only a captured payment can be refunded", payment.Status))
	}

	if err := ps.refund(ctx, payment, models.RefundReasonAdmin, actingUserID, note); err != nil {
		if errors.Is(err, apperrors.ErrRefundAlreadyExists) || errors.Is(err, apperrors.ErrPaymentNotModifiable) {
			return nil, err
		}
		return nil, fmt.Errorf("refund payment %d: %w", paymentID, err)
	}

	refunded, err := ps.payments.GetByID(ctx, paymentID)
	if err != nil {
		return nil, fmt.Errorf("get refunded payment: %w", err)
	}

	return refunded, nil
}

// ExpireUnpaid cancels up to limit bookings whose payment was due and never
// arrived, freeing their slots, and returns how many it cancelled. A booking
// paid meanwhile is skipped, so it is safe to run while payments come in.
func (ps *PaymentService) ExpireUnpaid(ctx context.Context, limit int) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("expire unpaid context: %w", err)
	}
	limit, _ = clampPagination(limit, 0)

	now := ps.now()
	due, err := ps.appointments.ListUnpaidDue(ctx, now, limit)
	if err != nil {
		return 0, fmt.Errorf("list unpaid appointments due: %w", err)
	}

	expired := 0
	for _, appointment := range due {
		err := ps.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
			if err := ps.appointments.CancelUnpaid(txCtx, appointment.ID, now); err != nil {
				return err
			}
			if err := ps.appointments.CreateStatusChange(txCtx, &models.AppointmentStatusChange{
				AppointmentID: appointment.ID,
				FromStatus:    models.AppointmentStatusScheduled,
				ToStatus:      models.AppointmentStatusCancelled,
				Reason:        "payment not received in time",
			}); err != nil {
				return err
			}

			cancelled := appointment.Appointment
			cancelled.Status = models.AppointmentStatusCancelled
			cancelled.CancelReason = models.CancelReasonPaymentExpired
			return ps.outbox.Create(txCtx, appointmentEvent(models.OutboxEventAppointmentCancelled, &cancelled, 0))
		})
		if errors.Is(err, apperrors.ErrAppointmentNotModifiable) {
			continue
		}
		if err != nil {
			return expired, fmt.Errorf("expire unpaid appointment %d: %w", appointment.ID, err)
		}
		expired++

		if err := ps.settleCancelled(ctx, appointment.ID, 0, models.RefundReasonLatePayment); err != nil {
			return expired, err
		}
	}

	return expired, nil
}

// settleCancelled closes the money side of a cancelled appointment: an open
// order expires and a captured payment is refunded in full.
func (ps *PaymentService) settleCancelled(
	ctx context.Context,
	appointmentID uint64,
	actingUserID uint64,
	reason models.RefundReason,
) error {
	payment, err := ps.payments.GetActiveByAppointment(ctx, appointmentID)
	if err != nil {
		if errors.Is(err, apperrors.ErrPaymentNotFound) {
			return nil
		}
		return fmt.Errorf("get active payment of cancelled appointment: %w", err)
	}

	if payment.Status == models.PaymentStatusCaptured {
		if err := ps.refund(ctx, payment, reason, actingUserID, ""); err != nil {
			return fmt.Errorf("refund cancelled appointment %d: %w", appointmentID, err)
		}
		return nil
	}

	err = ps.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := ps.payments.UpdateStatus(txCtx, payment.ID, models.PaymentStatusCreated, models.PaymentStatusExpired, "", nil); err != nil {
			return err
		}

		return ps.payments.CreateStatusChange(txCtx, &models.PaymentStatusChange{
			PaymentID:       payment.ID,
			ChangedByUserID: actingUserID,
			FromStatus:      models.PaymentStatusCreated,
			ToStatus:        models.PaymentStatusExpired,
			Reason:          "appointment cancelled",
		})
	})
	if err != nil && !errors.Is(err, apperrors.ErrPaymentNotModifiable) {
		return fmt.Errorf("expire payment of cancelled appointment %d: %w", appointmentID, err)
	}

	return nil
}

// moveToAppointment carries an appointment's payments to the appointment that
// replaces it when it is rescheduled. Callers run it inside a transaction.
func (ps *PaymentService) moveToAppointment(txCtx context.Context, fromAppointmentID uint64, toAppointmentID uint64) error {
	return ps.payments.MoveToAppointment(txCtx, fromAppointmentID, toAppointmentID)
}

// refund returns a captured payment in full. The refund is recorded as
// requested and committed before the gateway is called, then finished with the
// gateway's answer, so a gateway call never holds a transaction open.
// ponytail: a refund left requested by a crash between the steps needs a retry
// job; add it with the gateway webhooks in slice 6.
func (ps *PaymentService) refund(
	ctx context.Context,
	payment *models.Payment,
	reason models.RefundReason,
	actingUserID uint64,
	note string,
) error {
	requested, err := ps.payments.CreateRefund(ctx, &models.Refund{
		PaymentID:         payment.ID,
		InitiatedByUserID: actingUserID,
		Amount:            payment.Amount,
		Reason:            reason,
		Note:              note,
		Status:            models.RefundStatusRequested,
	})
	if err != nil {
		return err
	}

	providerRefundID, gatewayErr := ps.gateway.Refund(ctx, payment, payment.Amount)
	if gatewayErr != nil {
		if err := ps.payments.FinishRefund(ctx, requested.ID, models.RefundStatusFailed, ""); err != nil {
			return fmt.Errorf("record failed refund: %w", err)
		}
		return fmt.Errorf("refund with gateway: %w", gatewayErr)
	}

	return ps.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := ps.payments.FinishRefund(txCtx, requested.ID, models.RefundStatusSucceeded, providerRefundID); err != nil {
			return err
		}
		if err := ps.payments.UpdateStatus(txCtx, payment.ID, models.PaymentStatusCaptured, models.PaymentStatusRefunded, "", nil); err != nil {
			return err
		}
		if err := ps.payments.CreateStatusChange(txCtx, &models.PaymentStatusChange{
			PaymentID:       payment.ID,
			ChangedByUserID: actingUserID,
			FromStatus:      models.PaymentStatusCaptured,
			ToStatus:        models.PaymentStatusRefunded,
			Reason:          string(reason),
		}); err != nil {
			return err
		}
		if err := ps.appointments.SetPaymentStatus(txCtx, payment.AppointmentID, models.AppointmentPaymentPaid, models.AppointmentPaymentRefunded); err != nil &&
			!errors.Is(err, apperrors.ErrAppointmentNotModifiable) {
			return err
		}

		refunded := *payment
		refunded.Status = models.PaymentStatusRefunded
		return ps.outbox.Create(txCtx, paymentEvent(models.OutboxEventPaymentRefunded, &refunded))
	})
}

// payableAppointment loads an appointment for the patient side that can pay:
// an active link with full or appointments_only access. No link reads as not
// found, so appointment IDs are not revealed.
func (ps *PaymentService) payableAppointment(
	ctx context.Context,
	actingUserID uint64,
	appointmentID uint64,
) (*models.AppointmentDetails, error) {
	appointment, err := ps.appointments.GetByID(ctx, appointmentID)
	if err != nil {
		if errors.Is(err, apperrors.ErrAppointmentNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("get appointment by id: %w", err)
	}

	link, err := ps.userPatients.GetActiveByUserAndPatient(ctx, actingUserID, appointment.PatientID)
	if err != nil {
		if errors.Is(err, apperrors.ErrUserPatientNotFound) {
			return nil, apperrors.ErrAppointmentNotFound
		}
		return nil, fmt.Errorf("authorize payment: %w", err)
	}
	if !canBook(link) {
		return nil, apperrors.ErrPatientForbidden
	}

	return appointment, nil
}

func (ps *PaymentService) checkPayable(appointment *models.AppointmentDetails) error {
	if appointment.PaymentStatus != models.AppointmentPaymentUnpaid {
		return apperrors.Invalid(apperrors.ErrPaymentNotRequired, "", fmt.Sprintf("appointment payment is %s", appointment.PaymentStatus))
	}
	if appointment.Status != models.AppointmentStatusScheduled ||
		appointment.PaymentDueAt == nil || !ps.now().Before(*appointment.PaymentDueAt) {
		return apperrors.Invalid(apperrors.ErrPaymentWindowClosed, "", "the payment window for this booking has closed; book again")
	}

	return nil
}

func (ps *PaymentService) checkCanSee(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	appointment *models.AppointmentDetails,
) error {
	switch actingRole {
	case models.RoleAdmin:
		return nil
	case models.RoleDoctor:
		doctor, err := ps.doctors.GetByUserID(ctx, actingUserID)
		if err != nil && !errors.Is(err, apperrors.ErrDoctorNotFound) {
			return fmt.Errorf("get own doctor profile: %w", err)
		}
		if err == nil && doctor.ID == appointment.DoctorID {
			return nil
		}
	}

	if _, err := ps.userPatients.GetActiveByUserAndPatient(ctx, actingUserID, appointment.PatientID); err != nil {
		if errors.Is(err, apperrors.ErrUserPatientNotFound) {
			return apperrors.ErrAppointmentNotFound
		}
		return fmt.Errorf("authorize payment read: %w", err)
	}

	return nil
}

func paymentEvent(eventType string, payment *models.Payment) *models.OutboxEvent {
	return &models.OutboxEvent{
		AggregateType: models.OutboxAggregatePayment,
		AggregateID:   payment.ID,
		EventType:     eventType,
		Payload: map[string]any{
			"paymentId":     payment.ID,
			"appointmentId": payment.AppointmentID,
			"payerUserId":   payment.PayerUserID,
			"amount":        payment.Amount,
			"currency":      payment.Currency,
			"provider":      payment.Provider,
			"status":        payment.Status,
		},
	}
}
