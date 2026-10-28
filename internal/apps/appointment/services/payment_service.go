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
// it and marks the appointment paid in one transaction. signature is the proof
// the gateway's checkout gave the client. Verifying a captured payment again
// with the same payment id returns it unchanged. When the booking expired
// first, the money is refunded and ErrPaymentWindowClosed is returned.
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
	signature = strings.TrimSpace(signature)
	if signature == "" {
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
	paidAt, err := ps.gateway.VerifyPayment(ctx, payment, providerPaymentID, signature)
	if err != nil {
		if errors.Is(err, apperrors.ErrPaymentVerificationFailed) {
			return nil, err
		}
		return nil, fmt.Errorf("verify payment with gateway: %w", err)
	}

	return ps.capture(ctx, payment, appointment, actingUserID, providerPaymentID, paidAt)
}

// HandleWebhook applies a gateway callback. Money is recorded only after the
// gateway confirms it again, so a replayed or forged body captures nothing.
// Money reported for an order that was already closed is captured and refunded.
// A callback that changes nothing returns nil, so the gateway stops retrying;
// one with nothing to apply, such as an order this service never opened,
// returns ErrWebhookIgnored with the reason.
func (ps *PaymentService) HandleWebhook(ctx context.Context, body []byte, headers map[string][]string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("handle payment webhook context: %w", err)
	}

	event, err := ps.gateway.ParseWebhook(body, headers)
	if err != nil {
		if errors.Is(err, apperrors.ErrPaymentVerificationFailed) || errors.Is(err, apperrors.ErrWebhookIgnored) {
			return err
		}
		return fmt.Errorf("parse payment webhook: %w", err)
	}

	switch event.Type {
	case models.GatewayEventPaymentCaptured:
		payment, err := ps.payments.GetByProviderOrderID(ctx, ps.gateway.Provider(), event.ProviderOrderID)
		if err != nil {
			if errors.Is(err, apperrors.ErrPaymentNotFound) {
				return webhookIgnored("no payment has order %s", event.ProviderOrderID)
			}
			return fmt.Errorf("get payment by provider order id: %w", err)
		}
		return ps.captureReported(ctx, payment, event.ProviderPaymentID)
	case models.GatewayEventRefundProcessed, models.GatewayEventRefundFailed:
		return ps.settleReportedRefund(ctx, event)
	default:
		return webhookIgnored("event type %q is not handled", event.Type)
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
// permission decides who is an admin. The returned payment is still captured
// while the gateway processes the refund. A refund the gateway refused returns
// ErrRefundFailed; a failed refund can be asked for again.
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
		if errors.Is(err, apperrors.ErrRefundAlreadyExists) || errors.Is(err, apperrors.ErrPaymentNotModifiable) ||
			errors.Is(err, apperrors.ErrRefundFailed) {
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

// ListRefunds returns refunds in one status, oldest first. Requested refunds
// are still with the gateway. An empty status means requested.
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
// arrived, freeing their slots, and returns how many it cancelled. It first
// asks the gateway whether the open order was paid after all, in case its
// webhook was lost, and captures that payment instead. A booking paid
// meanwhile is skipped, so it is safe to run while payments come in.
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
		if err := ps.capturePaidOrder(ctx, appointment.ID); err != nil {
			return expired, err
		}

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

// ReconcileRefunds settles up to limit refunds still requested with what the
// gateway says about them, for webhooks that were lost, and returns how many
// it settled. A refund the gateway never received is failed after
// constant.RefundGatewayGrace, so an admin can retry it.
func (ps *PaymentService) ReconcileRefunds(ctx context.Context, limit int) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("reconcile refunds context: %w", err)
	}
	limit, _ = clampPagination(limit, 0)

	// ponytail: only the oldest page of requested refunds; page through them if
	// more than limit are ever waiting at once.
	refunds, err := ps.payments.ListRefunds(ctx, models.RefundStatusRequested, limit, 0)
	if err != nil {
		return 0, fmt.Errorf("list requested refunds: %w", err)
	}

	settled := 0
	for _, refund := range refunds {
		payment, err := ps.payments.GetByID(ctx, refund.PaymentID)
		if err != nil {
			return settled, fmt.Errorf("get payment of refund %d: %w", refund.ID, err)
		}
		providerRefundID, status, err := ps.gateway.RefundStatus(ctx, payment)
		if err != nil {
			return settled, fmt.Errorf("get gateway status of refund %d: %w", refund.ID, err)
		}
		if providerRefundID == "" {
			if ps.now().Sub(refund.CreatedAt) < constant.RefundGatewayGrace {
				continue
			}
			status = models.RefundStatusFailed
		}
		if status == models.RefundStatusRequested {
			continue
		}

		if err := ps.settleRefund(ctx, payment, refund, providerRefundID, status, 0); err != nil {
			return settled, fmt.Errorf("settle refund %d: %w", refund.ID, err)
		}
		settled++
	}

	return settled, nil
}

// capturePaidOrder captures the appointment's open payment when the gateway
// says its order was paid. An order that was not paid is left for expiry.
func (ps *PaymentService) capturePaidOrder(ctx context.Context, appointmentID uint64) error {
	payment, err := ps.payments.GetActiveByAppointment(ctx, appointmentID)
	if err != nil {
		if errors.Is(err, apperrors.ErrPaymentNotFound) {
			return nil
		}
		return fmt.Errorf("get active payment of unpaid appointment %d: %w", appointmentID, err)
	}
	if payment.Status != models.PaymentStatusCreated {
		return nil
	}

	providerPaymentID, err := ps.gateway.FindPaidOrder(ctx, payment)
	if err != nil {
		return fmt.Errorf("ask gateway about the order of appointment %d: %w", appointmentID, err)
	}
	if providerPaymentID == "" {
		return nil
	}

	if err := ps.captureReported(ctx, payment, providerPaymentID); err != nil && !errors.Is(err, apperrors.ErrWebhookIgnored) {
		return fmt.Errorf("capture paid order of appointment %d: %w", appointmentID, err)
	}

	return nil
}

// settleCancelled closes the money side of a cancelled appointment: an open
// order expires and a captured payment is refunded in full. A payment already
// being refunded, such as one that arrived late, is left to that refund.
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
		if err := ps.refund(ctx, payment, reason, actingUserID, ""); err != nil && !errors.Is(err, apperrors.ErrRefundAlreadyExists) {
			return fmt.Errorf("refund cancelled appointment %d: %w", appointmentID, err)
		}
		return nil
	}

	// The gateway's order stays open: money that still arrives is reported by
	// webhook, captured and refunded.
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
// appointment paid in one transaction. paidAt is when the payer paid, or zero
// for now. A payment made after the booking's payment was due, or for a booking
// no longer waiting for it, is captured and refunded instead, and
// ErrPaymentWindowClosed is returned.
func (ps *PaymentService) capture(
	ctx context.Context,
	payment *models.Payment,
	appointment *models.AppointmentDetails,
	actingUserID uint64,
	providerPaymentID string,
	paidAt time.Time,
) (*models.Payment, error) {
	now := ps.now()
	if paidAt.IsZero() {
		paidAt = now
	}
	late := appointment.Status != models.AppointmentStatusScheduled ||
		appointment.PaymentStatus != models.AppointmentPaymentUnpaid ||
		appointment.PaymentDueAt == nil || !paidAt.Before(*appointment.PaymentDueAt)
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

// captureReported captures money the gateway reported, by webhook or when
// asked. The order may already be closed, failed or expired, when the payer
// paid late; a captured or refunded payment means the report is a repeat.
func (ps *PaymentService) captureReported(ctx context.Context, payment *models.Payment, providerPaymentID string) error {
	switch payment.Status {
	case models.PaymentStatusCreated, models.PaymentStatusFailed, models.PaymentStatusExpired:
	default:
		return nil
	}
	paidAt, err := ps.gateway.VerifyPayment(ctx, payment, providerPaymentID, "")
	if err != nil {
		if errors.Is(err, apperrors.ErrPaymentVerificationFailed) {
			return webhookIgnored("the gateway does not confirm payment %d: %s", payment.ID, apperrors.Reason(err))
		}
		return fmt.Errorf("verify reported payment with gateway: %w", err)
	}

	appointment, err := ps.appointments.GetByID(ctx, payment.AppointmentID)
	if err != nil {
		return fmt.Errorf("get appointment of reported payment: %w", err)
	}
	if _, err := ps.capture(ctx, payment, appointment, 0, providerPaymentID, paidAt); err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPaymentWindowClosed), errors.Is(err, apperrors.ErrPaymentNotModifiable):
			return nil
		case errors.Is(err, apperrors.ErrPaymentVerificationFailed):
			return webhookIgnored("payment %d: %s", payment.ID, apperrors.Reason(err))
		}
		return err
	}

	return nil
}

// settleReportedRefund settles the refund a webhook reports. A refund already
// settled, or one this service did not ask for, changes nothing.
func (ps *PaymentService) settleReportedRefund(ctx context.Context, event *models.GatewayEvent) error {
	payment, err := ps.payments.GetByProviderPaymentID(ctx, ps.gateway.Provider(), event.ProviderPaymentID)
	if err != nil {
		if errors.Is(err, apperrors.ErrPaymentNotFound) {
			return webhookIgnored("no payment has gateway payment %s", event.ProviderPaymentID)
		}
		return fmt.Errorf("get payment by provider payment id: %w", err)
	}
	refund, err := ps.payments.GetRequestedRefundByPayment(ctx, payment.ID)
	if err != nil {
		if errors.Is(err, apperrors.ErrRefundNotFound) {
			return webhookIgnored("payment %d has no refund waiting", payment.ID)
		}
		return fmt.Errorf("get requested refund of payment %d: %w", payment.ID, err)
	}
	if event.Amount != refund.Amount {
		return webhookIgnored("gateway refund %s of %d paise does not match refund %d", event.ProviderRefundID, event.Amount, refund.ID)
	}

	status := models.RefundStatusSucceeded
	if event.Type == models.GatewayEventRefundFailed {
		status = models.RefundStatusFailed
	}

	return ps.settleRefund(ctx, payment, refund, event.ProviderRefundID, status, 0)
}

// refund returns a captured payment in full. The refund is recorded as
// requested and committed before the gateway is called, then settled with the
// gateway's answer, so a gateway call never holds a transaction open. A refund
// the gateway is still processing stays requested until its webhook or
// ReconcileRefunds settles it; so does one whose gateway call failed, since the
// gateway may have received it. A refund the gateway refused returns
// ErrRefundFailed.
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

	providerRefundID, status, err := ps.gateway.Refund(ctx, payment, requested)
	if err != nil {
		return fmt.Errorf("refund with gateway: %w", err)
	}
	if err := ps.settleRefund(ctx, payment, requested, providerRefundID, status, actingUserID); err != nil {
		return err
	}
	if status == models.RefundStatusFailed {
		return apperrors.ErrRefundFailed
	}

	return nil
}

// settleRefund records the gateway's final word on a requested refund. A
// succeeded refund marks the payment and its appointment refunded; a failed one
// tells admins through the outbox. A requested status changes nothing, and so
// does a refund that something else settled first.
func (ps *PaymentService) settleRefund(
	ctx context.Context,
	payment *models.Payment,
	refund *models.Refund,
	providerRefundID string,
	status models.RefundStatus,
	actingUserID uint64,
) error {
	var err error
	switch status {
	case models.RefundStatusSucceeded:
		err = ps.finishRefund(ctx, payment, refund.ID, providerRefundID, refund.Reason, actingUserID)
	case models.RefundStatusFailed:
		err = ps.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
			if err := ps.payments.FinishRefund(txCtx, refund.ID, models.RefundStatusFailed, providerRefundID); err != nil {
				return err
			}

			event := paymentEvent(models.OutboxEventRefundFailed, payment)
			event.Payload["refundId"] = refund.ID
			return ps.outbox.Create(txCtx, event)
		})
	}
	if errors.Is(err, apperrors.ErrPaymentNotModifiable) {
		return nil
	}

	return err
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

// webhookIgnored reports a gateway callback with nothing to apply and why.
func webhookIgnored(format string, args ...any) error {
	return apperrors.Invalid(apperrors.ErrWebhookIgnored, "", fmt.Sprintf(format, args...))
}
