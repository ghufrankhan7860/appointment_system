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
// What signature must hold is the gateway's call; some need none.
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

	return ps.capture(ctx, payment, appointment, actingUserID, providerPaymentID)
}

// HandleWebhook applies a gateway callback. Money is recorded only after the
// gateway confirms it again, so a replayed or forged body captures nothing.
// Money reported for an order that was already closed is captured and refunded.
// A callback that changes nothing returns nil, so the gateway stops retrying.
func (ps *PaymentService) HandleWebhook(ctx context.Context, body []byte, headers map[string][]string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("handle payment webhook context: %w", err)
	}

	event, err := ps.gateway.ParseWebhook(body, headers)
	if err != nil {
		if errors.Is(err, apperrors.ErrPaymentVerificationFailed) {
			return err
		}
		return fmt.Errorf("parse payment webhook: %w", err)
	}

	payment, err := ps.payments.GetByProviderOrderID(ctx, ps.gateway.Provider(), event.ProviderOrderID)
	if err != nil {
		if errors.Is(err, apperrors.ErrPaymentNotFound) {
			return err
		}
		return fmt.Errorf("get payment by provider order id: %w", err)
	}

	switch event.Status {
	case models.PaymentStatusCaptured:
		return ps.captureReported(ctx, payment, event)
	case models.PaymentStatusFailed, models.PaymentStatusExpired:
		err := ps.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
			if err := ps.payments.UpdateStatus(txCtx, payment.ID, models.PaymentStatusCreated, event.Status, "", nil); err != nil {
				return err
			}

			return ps.payments.CreateStatusChange(txCtx, &models.PaymentStatusChange{
				PaymentID:  payment.ID,
				FromStatus: models.PaymentStatusCreated,
				ToStatus:   event.Status,
				Reason:     "reported by the gateway",
			})
		})
		if err != nil && !errors.Is(err, apperrors.ErrPaymentNotModifiable) {
			return fmt.Errorf("close payment %d from webhook: %w", payment.ID, err)
		}
		return nil
	default:
		return nil
	}
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

// CompleteRefund records that an admin paid a requested refund back by hand,
// with the payout's reference such as a UPI UTR, and marks the payment and its
// appointment refunded.
func (ps *PaymentService) CompleteRefund(
	ctx context.Context,
	actingUserID uint64,
	refundID uint64,
	reference string,
) (*models.Refund, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("complete refund context: %w", err)
	}
	if actingUserID == 0 || refundID == 0 {
		return nil, fmt.Errorf("complete refund with missing ids: %w", apperrors.ErrInvalidPaymentInput)
	}
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return nil, apperrors.Invalid(apperrors.ErrInvalidPaymentInput, "reference", "is required")
	}
	if len(reference) > constant.MaxRefundReferenceLength {
		return nil, apperrors.Invalid(apperrors.ErrInvalidPaymentInput, "reference", fmt.Sprintf("must be at most %d characters", constant.MaxRefundReferenceLength))
	}

	refund, err := ps.payments.GetRefundByID(ctx, refundID)
	if err != nil {
		if errors.Is(err, apperrors.ErrRefundNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("get refund by id: %w", err)
	}
	if refund.Status != models.RefundStatusRequested {
		return nil, apperrors.Invalid(apperrors.ErrPaymentNotModifiable, "", fmt.Sprintf("refund is %s", refund.Status))
	}

	payment, err := ps.payments.GetByID(ctx, refund.PaymentID)
	if err != nil {
		return nil, fmt.Errorf("get refunded payment: %w", err)
	}
	if err := ps.finishRefund(ctx, payment, refund.ID, reference, refund.Reason, actingUserID); err != nil {
		if errors.Is(err, apperrors.ErrPaymentNotModifiable) {
			return nil, err
		}
		return nil, fmt.Errorf("complete refund %d: %w", refundID, err)
	}

	completed, err := ps.payments.GetRefundByID(ctx, refundID)
	if err != nil {
		return nil, fmt.Errorf("get completed refund: %w", err)
	}

	return completed, nil
}

// ListRefunds returns refunds in one status, oldest first. Requested refunds
// are the admin's queue of manual payouts. An empty status means requested.
func (ps *PaymentService) ListRefunds(
	ctx context.Context,
	status models.RefundStatus,
	limit int,
	offset int,
) ([]*models.Refund, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list refunds context: %w", err)
	}
	switch status {
	case "":
		status = models.RefundStatusRequested
	case models.RefundStatusRequested, models.RefundStatusSucceeded, models.RefundStatusFailed:
	default:
		return nil, apperrors.Invalid(apperrors.ErrInvalidPaymentInput, "status", "must be one of requested, succeeded, failed")
	}
	limit, offset = clampPagination(limit, offset)

	refunds, err := ps.payments.ListRefunds(ctx, status, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list refunds: %w", err)
	}

	return refunds, nil
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

	// Closing the gateway's order is best effort: money that still arrives is
	// reported by webhook, captured and refunded, so a failure here loses nothing.
	_ = ps.gateway.CancelOrder(ctx, payment)

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

// capture records a confirmed created payment as captured and marks its
// appointment paid in one transaction. A payment that arrived after the booking
// expired or was cancelled is captured and refunded instead, and
// ErrPaymentWindowClosed is returned.
func (ps *PaymentService) capture(
	ctx context.Context,
	payment *models.Payment,
	appointment *models.AppointmentDetails,
	actingUserID uint64,
	providerPaymentID string,
) (*models.Payment, error) {
	now := ps.now()
	late := appointment.Status != models.AppointmentStatusScheduled ||
		appointment.PaymentStatus != models.AppointmentPaymentUnpaid ||
		appointment.PaymentDueAt == nil || !now.Before(*appointment.PaymentDueAt)
	fromStatus := payment.Status

	err := ps.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := ps.payments.UpdateStatus(txCtx, payment.ID, fromStatus, models.PaymentStatusCaptured, providerPaymentID, &now); err != nil {
			return err
		}
		if err := ps.payments.CreateStatusChange(txCtx, &models.PaymentStatusChange{
			PaymentID:       payment.ID,
			ChangedByUserID: actingUserID,
			FromStatus:      fromStatus,
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

		captured := *payment
		captured.Status = models.PaymentStatusCaptured
		return ps.outbox.Create(txCtx, paymentEvent(models.OutboxEventPaymentCaptured, &captured))
	})
	if err != nil {
		if errors.Is(err, apperrors.ErrProviderPaymentUsed) {
			return nil, apperrors.Invalid(apperrors.ErrPaymentVerificationFailed, "providerPaymentId", "was already used for another payment")
		}
		if errors.Is(err, apperrors.ErrPaymentNotModifiable) {
			return nil, err
		}
		return nil, fmt.Errorf("capture payment %d: %w", payment.ID, err)
	}

	captured, err := ps.payments.GetByID(ctx, payment.ID)
	if err != nil {
		return nil, fmt.Errorf("get captured payment: %w", err)
	}
	if late {
		if err := ps.refund(ctx, captured, models.RefundReasonLatePayment, 0, ""); err != nil {
			return nil, fmt.Errorf("refund late payment %d: %w", payment.ID, err)
		}
		return nil, apperrors.Invalid(apperrors.ErrPaymentWindowClosed, "", "the booking expired before the payment arrived; the payment is being refunded")
	}

	return captured, nil
}

// captureReported captures money a webhook reports. The order may already be
// closed, failed or expired, when the payer paid late; a captured or refunded
// payment means the callback is a repeat.
func (ps *PaymentService) captureReported(ctx context.Context, payment *models.Payment, event *models.GatewayEvent) error {
	switch payment.Status {
	case models.PaymentStatusCreated, models.PaymentStatusFailed, models.PaymentStatusExpired:
	default:
		return nil
	}
	if event.Amount != payment.Amount {
		return apperrors.Invalid(apperrors.ErrPaymentVerificationFailed, "", "the reported amount differs from the payment")
	}
	if err := ps.gateway.VerifyPayment(ctx, payment, event.ProviderPaymentID, ""); err != nil {
		if errors.Is(err, apperrors.ErrPaymentVerificationFailed) {
			return err
		}
		return fmt.Errorf("verify reported payment with gateway: %w", err)
	}

	appointment, err := ps.appointments.GetByID(ctx, payment.AppointmentID)
	if err != nil {
		return fmt.Errorf("get appointment of reported payment: %w", err)
	}
	if _, err := ps.capture(ctx, payment, appointment, 0, event.ProviderPaymentID); err != nil {
		if errors.Is(err, apperrors.ErrPaymentWindowClosed) || errors.Is(err, apperrors.ErrPaymentNotModifiable) {
			return nil
		}
		return err
	}

	return nil
}

// refund returns a captured payment in full. The refund is recorded as
// requested and committed before the gateway is called, then finished with the
// gateway's answer, so a gateway call never holds a transaction open. A refund
// the gateway cannot make, or one left requested by a crash between the steps,
// waits in ListRefunds for an admin to pay back and complete.
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
	if errors.Is(gatewayErr, apperrors.ErrRefundManual) {
		event := paymentEvent(models.OutboxEventRefundManual, payment)
		event.Payload["refundId"] = requested.ID
		return ps.outbox.Create(ctx, event)
	}
	if gatewayErr != nil {
		if err := ps.payments.FinishRefund(ctx, requested.ID, models.RefundStatusFailed, ""); err != nil {
			return fmt.Errorf("record failed refund: %w", err)
		}
		return fmt.Errorf("refund with gateway: %w", gatewayErr)
	}

	return ps.finishRefund(ctx, payment, requested.ID, providerRefundID, reason, actingUserID)
}

// finishRefund marks a requested refund succeeded and the payment and its
// appointment refunded in one transaction.
func (ps *PaymentService) finishRefund(
	ctx context.Context,
	payment *models.Payment,
	refundID uint64,
	providerRefundID string,
	reason models.RefundReason,
	actingUserID uint64,
) error {
	return ps.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := ps.payments.FinishRefund(txCtx, refundID, models.RefundStatusSucceeded, providerRefundID); err != nil {
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
