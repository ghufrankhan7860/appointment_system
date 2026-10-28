package services_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
)

func TestPaymentServiceHandleWebhook(t *testing.T) {
	location := clinicLocation(t)
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, location)
	nineAM := time.Date(2026, 10, 26, 9, 0, 0, 0, location)
	paymentIn := func(status models.PaymentStatus) *models.Payment {
		return &models.Payment{ID: 70, AppointmentID: 50, PayerUserID: 7, Amount: 40000, Currency: constant.PaymentCurrency, Status: status, ProviderOrderID: "order-1"}
	}
	captured := func() *models.GatewayEvent {
		return &models.GatewayEvent{Type: models.GatewayEventPaymentCaptured, ProviderOrderID: "order-1", ProviderPaymentID: "pay_1", Amount: 40000}
	}
	cancelled := unpaidAt(nineAM, now.Add(5*time.Minute))
	cancelled.Status = models.AppointmentStatusCancelled
	unknownOrder := captured()
	unknownOrder.ProviderOrderID = "order-other"

	tests := []struct {
		name          string
		payment       *models.Payment
		appointment   *models.AppointmentDetails
		event         *models.GatewayEvent
		parseErr      error
		verifyErr     error
		paidAt        time.Time
		wantErr       error
		wantStatus    models.PaymentStatus
		wantMarkPaid  bool
		wantRefunded  int
		wantOutboxEvt string
	}{
		{name: "a reported capture captures and marks the booking paid", payment: paymentIn(models.PaymentStatusCreated), event: captured(), wantStatus: models.PaymentStatusCaptured, wantMarkPaid: true, wantOutboxEvt: models.OutboxEventPaymentCaptured},
		{name: "a replayed capture changes nothing", payment: paymentIn(models.PaymentStatusCaptured), event: captured(), wantStatus: models.PaymentStatusCaptured},
		{name: "a bad signature is refused", payment: paymentIn(models.PaymentStatusCreated), parseErr: apperrors.ErrPaymentVerificationFailed, wantErr: apperrors.ErrPaymentVerificationFailed, wantStatus: models.PaymentStatusCreated},
		{name: "an event with nothing to apply is ignored", payment: paymentIn(models.PaymentStatusCreated), parseErr: apperrors.Invalid(apperrors.ErrWebhookIgnored, "", "event is not used"), wantErr: apperrors.ErrWebhookIgnored, wantStatus: models.PaymentStatusCreated},
		{name: "a capture the gateway does not confirm is ignored", payment: paymentIn(models.PaymentStatusCreated), event: captured(), verifyErr: apperrors.ErrPaymentVerificationFailed, wantErr: apperrors.ErrWebhookIgnored, wantStatus: models.PaymentStatusCreated},
		{name: "money for a cancelled booking is captured and refunded", payment: paymentIn(models.PaymentStatusExpired), appointment: cancelled, event: captured(), wantStatus: models.PaymentStatusRefunded, wantRefunded: 40000, wantOutboxEvt: models.OutboxEventPaymentRefunded},
		{name: "a payment made in time but reported after the hold keeps the booking", payment: paymentIn(models.PaymentStatusCreated), appointment: unpaidAt(nineAM, now.Add(-time.Minute)), event: captured(), paidAt: now.Add(-2 * time.Minute), wantStatus: models.PaymentStatusCaptured, wantMarkPaid: true, wantOutboxEvt: models.OutboxEventPaymentCaptured},
		{name: "a payment made after the hold is refunded", payment: paymentIn(models.PaymentStatusCreated), appointment: unpaidAt(nineAM, now.Add(-time.Minute)), event: captured(), paidAt: now.Add(-30 * time.Second), wantStatus: models.PaymentStatusRefunded, wantRefunded: 40000, wantOutboxEvt: models.OutboxEventPaymentRefunded},
		{name: "an order this service never opened is ignored", payment: paymentIn(models.PaymentStatusCreated), event: unknownOrder, wantErr: apperrors.ErrWebhookIgnored, wantStatus: models.PaymentStatusCreated},
		{name: "an event type with no handler is ignored", payment: paymentIn(models.PaymentStatusCreated), event: &models.GatewayEvent{Type: "dispute_created"}, wantErr: apperrors.ErrWebhookIgnored, wantStatus: models.PaymentStatusCreated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.appointment == nil {
				tt.appointment = unpaidAt(nineAM, now.Add(5*time.Minute))
			}
			appointments := &fakeAppointmentRepo{byID: tt.appointment}
			payments := &fakePaymentRepo{payment: tt.payment}
			gateway := &fakeGateway{event: tt.event, parseErr: tt.parseErr, verifyErr: tt.verifyErr, paidAt: tt.paidAt}
			outbox := &fakeOutboxRepo{}
			_, service := newTestAppointmentAndPaymentServices(t, &fakeDoctorRepo{}, appointments, outbox, &fakeUserPatientRepo{}, payments, gateway, now)

			err := service.HandleWebhook(context.Background(), []byte(`{}`), map[string][]string{})
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("HandleWebhook error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && err != nil {
				t.Fatalf("HandleWebhook returned error: %v", err)
			}
			if payments.payment.Status != tt.wantStatus {
				t.Fatalf("HandleWebhook left the payment %q, want %q", payments.payment.Status, tt.wantStatus)
			}
			if appointments.markPaidCalled != tt.wantMarkPaid || gateway.refunded != tt.wantRefunded {
				t.Fatalf("HandleWebhook marked paid %v and refunded %d, want %v and %d", appointments.markPaidCalled, gateway.refunded, tt.wantMarkPaid, tt.wantRefunded)
			}
			if tt.wantOutboxEvt == "" && len(outbox.events) != 0 || tt.wantOutboxEvt != "" && (len(outbox.events) != 1 || outbox.events[0].EventType != tt.wantOutboxEvt) {
				t.Fatalf("HandleWebhook outbox = %+v, want %q", outbox.events, tt.wantOutboxEvt)
			}
			if tt.wantMarkPaid && (payments.payment.ProviderPaymentID != "pay_1" || payments.changes[0].ChangedByUserID != 0) {
				t.Fatalf("HandleWebhook captured with %q by %d, want the reported payment by the system", payments.payment.ProviderPaymentID, payments.changes[0].ChangedByUserID)
			}
		})
	}
}

func TestPaymentServiceRefundWebhook(t *testing.T) {
	refundEvent := func(eventType models.GatewayEventType) *models.GatewayEvent {
		return &models.GatewayEvent{Type: eventType, ProviderPaymentID: "pay_1", ProviderRefundID: "rfnd_1", Amount: 40000}
	}
	wrongAmount := refundEvent(models.GatewayEventRefundProcessed)
	wrongAmount.Amount = 100
	otherPayment := refundEvent(models.GatewayEventRefundProcessed)
	otherPayment.ProviderPaymentID = "pay_other"

	tests := []struct {
		name           string
		refundStatus   models.RefundStatus
		event          *models.GatewayEvent
		wantErr        error
		wantRefund     models.RefundStatus
		wantPayment    models.PaymentStatus
		wantOutboxEvt  string
		wantProviderID string
	}{
		{name: "a processed refund marks the payment refunded", refundStatus: models.RefundStatusRequested, event: refundEvent(models.GatewayEventRefundProcessed), wantRefund: models.RefundStatusSucceeded, wantPayment: models.PaymentStatusRefunded, wantOutboxEvt: models.OutboxEventPaymentRefunded, wantProviderID: "rfnd_1"},
		{name: "a failed refund tells admins and keeps the payment captured", refundStatus: models.RefundStatusRequested, event: refundEvent(models.GatewayEventRefundFailed), wantRefund: models.RefundStatusFailed, wantPayment: models.PaymentStatusCaptured, wantOutboxEvt: models.OutboxEventRefundFailed, wantProviderID: "rfnd_1"},
		{name: "a repeat for a settled refund is ignored", refundStatus: models.RefundStatusSucceeded, event: refundEvent(models.GatewayEventRefundProcessed), wantErr: apperrors.ErrWebhookIgnored, wantRefund: models.RefundStatusSucceeded, wantPayment: models.PaymentStatusCaptured},
		{name: "a refund of another amount is ignored", refundStatus: models.RefundStatusRequested, event: wrongAmount, wantErr: apperrors.ErrWebhookIgnored, wantRefund: models.RefundStatusRequested, wantPayment: models.PaymentStatusCaptured},
		{name: "a refund of an unknown payment is ignored", refundStatus: models.RefundStatusRequested, event: otherPayment, wantErr: apperrors.ErrWebhookIgnored, wantRefund: models.RefundStatusRequested, wantPayment: models.PaymentStatusCaptured},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payments := &fakePaymentRepo{
				payment: &models.Payment{ID: 70, AppointmentID: 50, Amount: 40000, ProviderPaymentID: "pay_1", Status: models.PaymentStatusCaptured},
				refunds: []*models.Refund{{ID: 90, PaymentID: 70, Amount: 40000, Reason: models.RefundReasonDoctorCancelled, Status: tt.refundStatus}},
			}
			outbox := &fakeOutboxRepo{}
			_, service := newTestAppointmentAndPaymentServices(t, &fakeDoctorRepo{}, &fakeAppointmentRepo{}, outbox, &fakeUserPatientRepo{}, payments, &fakeGateway{event: tt.event}, time.Now())

			err := service.HandleWebhook(context.Background(), []byte(`{}`), map[string][]string{})
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("HandleWebhook error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && err != nil {
				t.Fatalf("HandleWebhook returned error: %v", err)
			}
			if payments.refunds[0].Status != tt.wantRefund || payments.payment.Status != tt.wantPayment {
				t.Fatalf("HandleWebhook left the refund %q and the payment %q, want %q and %q", payments.refunds[0].Status, payments.payment.Status, tt.wantRefund, tt.wantPayment)
			}
			if tt.wantProviderID != "" && payments.refunds[0].ProviderRefundID != tt.wantProviderID {
				t.Fatalf("HandleWebhook recorded gateway refund %q, want %q", payments.refunds[0].ProviderRefundID, tt.wantProviderID)
			}
			if tt.wantOutboxEvt == "" && len(outbox.events) != 0 || tt.wantOutboxEvt != "" && (len(outbox.events) != 1 || outbox.events[0].EventType != tt.wantOutboxEvt) {
				t.Fatalf("HandleWebhook outbox = %+v, want %q", outbox.events, tt.wantOutboxEvt)
			}
			if tt.wantOutboxEvt == models.OutboxEventRefundFailed && outbox.events[0].Payload["refundId"] != uint64(90) {
				t.Fatalf("HandleWebhook refund.failed payload = %+v, want refundId 90", outbox.events[0].Payload)
			}
		})
	}
}

func TestPaymentServiceAsyncRefund(t *testing.T) {
	newPayments := func() *fakePaymentRepo {
		return &fakePaymentRepo{payment: &models.Payment{ID: 70, AppointmentID: 50, Amount: 40000, ProviderPaymentID: "pay_1", Status: models.PaymentStatusCaptured}}
	}

	t.Run("a refund the gateway is processing stays requested", func(t *testing.T) {
		payments := newPayments()
		outbox := &fakeOutboxRepo{}
		_, service := newTestAppointmentAndPaymentServices(t, &fakeDoctorRepo{}, &fakeAppointmentRepo{}, outbox, &fakeUserPatientRepo{}, payments, &fakeGateway{refundStatus: models.RefundStatusRequested}, time.Now())

		refunded, err := service.Refund(context.Background(), 1, 70, "")
		if err != nil {
			t.Fatalf("Refund returned error: %v", err)
		}
		if refunded.Status != models.PaymentStatusCaptured || len(payments.finished) != 0 || payments.refunds[0].Status != models.RefundStatusRequested || len(outbox.events) != 0 {
			t.Fatalf("Refund left %q with finished %v and outbox %+v, want captured with the refund still requested", refunded.Status, payments.finished, outbox.events)
		}
	})

	t.Run("a refund the gateway refuses is failed and admins are told", func(t *testing.T) {
		payments := newPayments()
		outbox := &fakeOutboxRepo{}
		_, service := newTestAppointmentAndPaymentServices(t, &fakeDoctorRepo{}, &fakeAppointmentRepo{}, outbox, &fakeUserPatientRepo{}, payments, &fakeGateway{refundStatus: models.RefundStatusFailed}, time.Now())

		if _, err := service.Refund(context.Background(), 1, 70, ""); !errors.Is(err, apperrors.ErrRefundFailed) {
			t.Fatalf("Refund error = %v, want ErrRefundFailed", err)
		}
		if payments.payment.Status != models.PaymentStatusCaptured || payments.refunds[0].Status != models.RefundStatusFailed {
			t.Fatalf("Refund left the payment %q and the refund %q, want captured and failed", payments.payment.Status, payments.refunds[0].Status)
		}
		if len(outbox.events) != 1 || outbox.events[0].EventType != models.OutboxEventRefundFailed {
			t.Fatalf("Refund outbox = %+v, want one refund.failed event", outbox.events)
		}
	})
}

func TestPaymentServiceExpireUnpaidAsksGateway(t *testing.T) {
	location := clinicLocation(t)
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, location)
	nineAM := time.Date(2026, 10, 26, 9, 0, 0, 0, location)
	errGatewayDown := errors.New("gateway down")

	tests := []struct {
		name            string
		gateway         *fakeGateway
		cancelUnpaidErr error
		wantErr         error
		wantExpired     int
		wantStatus      models.PaymentStatus
		wantMarkPaid    bool
		wantRefunded    int
	}{
		// The real repository refuses to cancel a booking that was just paid.
		{name: "an order paid in time whose webhook was lost is captured", gateway: &fakeGateway{paidOrder: "pay_1", paidAt: now.Add(-3 * time.Minute)}, cancelUnpaidErr: apperrors.ErrAppointmentNotModifiable, wantStatus: models.PaymentStatusCaptured, wantMarkPaid: true},
		{name: "an order paid after the hold is captured, refunded and expired", gateway: &fakeGateway{paidOrder: "pay_1", paidAt: now.Add(-30 * time.Second)}, wantExpired: 1, wantStatus: models.PaymentStatusRefunded, wantRefunded: 40000},
		{name: "an unpaid order expires", gateway: &fakeGateway{}, wantExpired: 1, wantStatus: models.PaymentStatusExpired},
		{name: "a gateway failure cancels nothing", gateway: &fakeGateway{findErr: errGatewayDown}, wantErr: errGatewayDown, wantStatus: models.PaymentStatusCreated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			booking := unpaidAt(nineAM, now.Add(-time.Minute))
			appointments := &fakeAppointmentRepo{byID: booking, unpaidDue: []*models.AppointmentDetails{booking}, cancelUnpaidErr: tt.cancelUnpaidErr}
			payments := &fakePaymentRepo{payment: &models.Payment{ID: 70, AppointmentID: 50, Amount: 40000, ProviderOrderID: "order-1", Status: models.PaymentStatusCreated}}
			_, service := newTestAppointmentAndPaymentServices(t, &fakeDoctorRepo{}, appointments, &fakeOutboxRepo{}, &fakeUserPatientRepo{}, payments, tt.gateway, now)

			expired, err := service.ExpireUnpaid(context.Background(), 10)
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("ExpireUnpaid error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && err != nil {
				t.Fatalf("ExpireUnpaid returned error: %v", err)
			}
			if expired != tt.wantExpired || tt.gateway.asked != 1 {
				t.Fatalf("ExpireUnpaid = %d after asking the gateway %d times, want %d after once", expired, tt.gateway.asked, tt.wantExpired)
			}
			if payments.payment.Status != tt.wantStatus || appointments.markPaidCalled != tt.wantMarkPaid || tt.gateway.refunded != tt.wantRefunded {
				t.Fatalf("ExpireUnpaid left %q, marked paid %v, refunded %d; want %q, %v, %d", payments.payment.Status, appointments.markPaidCalled, tt.gateway.refunded, tt.wantStatus, tt.wantMarkPaid, tt.wantRefunded)
			}
			if tt.wantErr != nil && len(appointments.cancelUnpaidIDs) != 0 {
				t.Fatalf("ExpireUnpaid cancelled %v after a gateway failure, want nothing", appointments.cancelUnpaidIDs)
			}
		})
	}
}

func TestPaymentServiceReconcileRefunds(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	errGatewayDown := errors.New("gateway down")

	tests := []struct {
		name          string
		createdAt     time.Time
		gateway       *fakeGateway
		wantErr       error
		wantSettled   int
		wantRefund    models.RefundStatus
		wantPayment   models.PaymentStatus
		wantOutboxEvt string
	}{
		{name: "a refund the gateway processed is finished", createdAt: now.Add(-time.Hour), gateway: &fakeGateway{gatewayRefundID: "rfnd_1", gatewayRefundStatus: models.RefundStatusSucceeded}, wantSettled: 1, wantRefund: models.RefundStatusSucceeded, wantPayment: models.PaymentStatusRefunded, wantOutboxEvt: models.OutboxEventPaymentRefunded},
		{name: "a refund the gateway failed is failed", createdAt: now.Add(-time.Hour), gateway: &fakeGateway{gatewayRefundID: "rfnd_1", gatewayRefundStatus: models.RefundStatusFailed}, wantSettled: 1, wantRefund: models.RefundStatusFailed, wantPayment: models.PaymentStatusCaptured, wantOutboxEvt: models.OutboxEventRefundFailed},
		{name: "a refund still processing is left", createdAt: now.Add(-time.Hour), gateway: &fakeGateway{gatewayRefundID: "rfnd_1", gatewayRefundStatus: models.RefundStatusRequested}, wantRefund: models.RefundStatusRequested, wantPayment: models.PaymentStatusCaptured},
		{name: "a recent refund the gateway has not seen is left", createdAt: now.Add(-time.Minute), gateway: &fakeGateway{}, wantRefund: models.RefundStatusRequested, wantPayment: models.PaymentStatusCaptured},
		{name: "an old refund the gateway never received is failed", createdAt: now.Add(-constant.RefundGatewayGrace - time.Minute), gateway: &fakeGateway{}, wantSettled: 1, wantRefund: models.RefundStatusFailed, wantPayment: models.PaymentStatusCaptured, wantOutboxEvt: models.OutboxEventRefundFailed},
		{name: "a gateway failure stops the run", createdAt: now.Add(-time.Hour), gateway: &fakeGateway{refundStatusErr: errGatewayDown}, wantErr: errGatewayDown, wantRefund: models.RefundStatusRequested, wantPayment: models.PaymentStatusCaptured},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payments := &fakePaymentRepo{
				payment: &models.Payment{ID: 70, AppointmentID: 50, Amount: 40000, ProviderPaymentID: "pay_1", Status: models.PaymentStatusCaptured},
				refunds: []*models.Refund{{ID: 90, PaymentID: 70, Amount: 40000, Reason: models.RefundReasonAdmin, Status: models.RefundStatusRequested, CreatedAt: tt.createdAt}},
			}
			outbox := &fakeOutboxRepo{}
			_, service := newTestAppointmentAndPaymentServices(t, &fakeDoctorRepo{}, &fakeAppointmentRepo{}, outbox, &fakeUserPatientRepo{}, payments, tt.gateway, now)

			settled, err := service.ReconcileRefunds(context.Background(), 10)
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("ReconcileRefunds error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && err != nil {
				t.Fatalf("ReconcileRefunds returned error: %v", err)
			}
			if settled != tt.wantSettled || payments.refunds[0].Status != tt.wantRefund || payments.payment.Status != tt.wantPayment {
				t.Fatalf("ReconcileRefunds settled %d, left the refund %q and the payment %q; want %d, %q, %q", settled, payments.refunds[0].Status, payments.payment.Status, tt.wantSettled, tt.wantRefund, tt.wantPayment)
			}
			if tt.wantOutboxEvt == "" && len(outbox.events) != 0 || tt.wantOutboxEvt != "" && (len(outbox.events) != 1 || outbox.events[0].EventType != tt.wantOutboxEvt) {
				t.Fatalf("ReconcileRefunds outbox = %+v, want %q", outbox.events, tt.wantOutboxEvt)
			}
		})
	}
}

func TestPaymentServiceListRefunds(t *testing.T) {
	refunds := []*models.Refund{{ID: 90, Status: models.RefundStatusRequested}, {ID: 91, Status: models.RefundStatusSucceeded}}

	tests := []struct {
		name       string
		status     models.RefundStatus
		wantErr    error
		wantStatus models.RefundStatus
	}{
		{name: "no status lists the refunds still with the gateway", wantStatus: models.RefundStatusRequested},
		{name: "succeeded refunds", status: models.RefundStatusSucceeded, wantStatus: models.RefundStatusSucceeded},
		{name: "an unknown status", status: "pending", wantErr: apperrors.ErrInvalidPaymentInput},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payments := &fakePaymentRepo{refunds: refunds}
			_, service := newTestAppointmentAndPaymentServices(t, &fakeDoctorRepo{}, &fakeAppointmentRepo{}, &fakeOutboxRepo{}, &fakeUserPatientRepo{}, payments, &fakeGateway{}, time.Now())

			listed, err := service.ListRefunds(context.Background(), tt.status, 0, 0)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ListRefunds error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ListRefunds returned error: %v", err)
			}
			if len(listed) != 1 || listed[0].Status != tt.wantStatus || payments.listedStatus != tt.wantStatus || payments.listedLimit != constant.DefaultListLimit {
				t.Fatalf("ListRefunds = %d rows for %q with limit %d, want 1 %q row with the default limit", len(listed), payments.listedStatus, payments.listedLimit, tt.wantStatus)
			}
		})
	}
}
