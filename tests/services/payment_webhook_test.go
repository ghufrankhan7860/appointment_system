package services_test

import (
	"context"
	"errors"
	"strings"
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
	reported := func(status models.PaymentStatus) *models.GatewayEvent {
		return &models.GatewayEvent{ProviderOrderID: "order-1", ProviderPaymentID: "utr-1", Amount: 40000, Status: status}
	}
	cancelled := unpaidAt(nineAM, now.Add(5*time.Minute))
	cancelled.Status = models.AppointmentStatusCancelled
	wrongAmount := reported(models.PaymentStatusCaptured)
	wrongAmount.Amount = 100
	unknownOrder := reported(models.PaymentStatusCaptured)
	unknownOrder.ProviderOrderID = "order-other"

	tests := []struct {
		name          string
		payment       *models.Payment
		appointment   *models.AppointmentDetails
		event         *models.GatewayEvent
		parseErr      error
		verifyErr     error
		wantErr       error
		wantStatus    models.PaymentStatus
		wantMarkPaid  bool
		wantRefunded  int
		wantOutboxEvt string
	}{
		{name: "a reported success captures and marks the booking paid", payment: paymentIn(models.PaymentStatusCreated), event: reported(models.PaymentStatusCaptured), wantStatus: models.PaymentStatusCaptured, wantMarkPaid: true, wantOutboxEvt: models.OutboxEventPaymentCaptured},
		{name: "a replayed success changes nothing", payment: paymentIn(models.PaymentStatusCaptured), event: reported(models.PaymentStatusCaptured), wantStatus: models.PaymentStatusCaptured},
		{name: "a bad signature is refused", payment: paymentIn(models.PaymentStatusCreated), parseErr: apperrors.ErrPaymentVerificationFailed, wantErr: apperrors.ErrPaymentVerificationFailed, wantStatus: models.PaymentStatusCreated},
		{name: "a success the gateway does not confirm is refused", payment: paymentIn(models.PaymentStatusCreated), event: reported(models.PaymentStatusCaptured), verifyErr: apperrors.ErrPaymentVerificationFailed, wantErr: apperrors.ErrPaymentVerificationFailed, wantStatus: models.PaymentStatusCreated},
		{name: "a different amount is refused", payment: paymentIn(models.PaymentStatusCreated), event: wrongAmount, wantErr: apperrors.ErrPaymentVerificationFailed, wantStatus: models.PaymentStatusCreated},
		{name: "money for a cancelled booking is captured and refunded", payment: paymentIn(models.PaymentStatusExpired), appointment: cancelled, event: reported(models.PaymentStatusCaptured), wantStatus: models.PaymentStatusRefunded, wantRefunded: 40000, wantOutboxEvt: models.OutboxEventPaymentRefunded},
		{name: "a failed attempt closes the order", payment: paymentIn(models.PaymentStatusCreated), event: reported(models.PaymentStatusFailed), wantStatus: models.PaymentStatusFailed},
		{name: "an expired session closes the order", payment: paymentIn(models.PaymentStatusCreated), event: reported(models.PaymentStatusExpired), wantStatus: models.PaymentStatusExpired},
		{name: "an expiry after capture changes nothing", payment: paymentIn(models.PaymentStatusCaptured), event: reported(models.PaymentStatusExpired), wantStatus: models.PaymentStatusCaptured},
		{name: "an event with no outcome changes nothing", payment: paymentIn(models.PaymentStatusCreated), event: reported(""), wantStatus: models.PaymentStatusCreated},
		{name: "an unknown order", payment: paymentIn(models.PaymentStatusCreated), event: unknownOrder, wantErr: apperrors.ErrPaymentNotFound, wantStatus: models.PaymentStatusCreated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.appointment == nil {
				tt.appointment = unpaidAt(nineAM, now.Add(5*time.Minute))
			}
			appointments := &fakeAppointmentRepo{byID: tt.appointment}
			payments := &fakePaymentRepo{payment: tt.payment}
			gateway := &fakeGateway{event: tt.event, parseErr: tt.parseErr, verifyErr: tt.verifyErr}
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
			if tt.wantMarkPaid && (payments.payment.ProviderPaymentID != "utr-1" || payments.changes[0].ChangedByUserID != 0) {
				t.Fatalf("HandleWebhook captured with %q by %d, want the reported utr by the system", payments.payment.ProviderPaymentID, payments.changes[0].ChangedByUserID)
			}
		})
	}
}

func TestPaymentServiceManualRefund(t *testing.T) {
	location := clinicLocation(t)
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, location)
	nineAM := time.Date(2026, 10, 26, 9, 0, 0, 0, location)

	t.Run("a gateway without refunds leaves the refund requested for an admin", func(t *testing.T) {
		payments := &fakePaymentRepo{payment: &models.Payment{ID: 70, AppointmentID: 50, Amount: 40000, Status: models.PaymentStatusCaptured}}
		outbox := &fakeOutboxRepo{}
		_, service := newTestAppointmentAndPaymentServices(t, &fakeDoctorRepo{}, &fakeAppointmentRepo{}, outbox, &fakeUserPatientRepo{}, payments, &fakeGateway{refundErr: apperrors.ErrRefundManual}, now)

		refunded, err := service.Refund(context.Background(), 1, 70, "")
		if err != nil {
			t.Fatalf("Refund returned error: %v", err)
		}
		if refunded.Status != models.PaymentStatusCaptured || len(payments.finished) != 0 || payments.refunds[0].Status != models.RefundStatusRequested {
			t.Fatalf("Refund left %q with finished %v, want captured with the refund still requested", refunded.Status, payments.finished)
		}
		if len(outbox.events) != 1 || outbox.events[0].EventType != models.OutboxEventRefundManual || outbox.events[0].Payload["refundId"] != uint64(90) {
			t.Fatalf("Refund outbox = %+v, want one manual refund event for refund 90", outbox.events)
		}
	})

	t.Run("an expired booking cancels its open gateway order even when the gateway fails", func(t *testing.T) {
		appointments := &fakeAppointmentRepo{unpaidDue: []*models.AppointmentDetails{unpaidAt(nineAM, now.Add(-time.Minute))}}
		payments := &fakePaymentRepo{payment: &models.Payment{ID: 70, AppointmentID: 50, Status: models.PaymentStatusCreated}}
		gateway := &fakeGateway{}
		_, service := newTestAppointmentAndPaymentServices(t, &fakeDoctorRepo{}, appointments, &fakeOutboxRepo{}, &fakeUserPatientRepo{}, payments, gateway, now)

		if expired, err := service.ExpireUnpaid(context.Background(), 10); err != nil || expired != 1 {
			t.Fatalf("ExpireUnpaid = %d, %v; want 1", expired, err)
		}
		if gateway.cancelled != 1 || payments.payment.Status != models.PaymentStatusExpired {
			t.Fatalf("ExpireUnpaid cancelled %d orders and left %q, want 1 and expired", gateway.cancelled, payments.payment.Status)
		}
	})
}

func TestPaymentServiceCompleteRefund(t *testing.T) {
	requested := func(status models.RefundStatus) []*models.Refund {
		return []*models.Refund{{ID: 90, PaymentID: 70, Amount: 40000, Reason: models.RefundReasonDoctorCancelled, Status: status}}
	}

	tests := []struct {
		name      string
		refunds   []*models.Refund
		reference string
		wantErr   error
	}{
		{name: "an admin records the payout", refunds: requested(models.RefundStatusRequested), reference: " 412345678901 "},
		{name: "a refund already finished", refunds: requested(models.RefundStatusSucceeded), reference: "412345678901", wantErr: apperrors.ErrPaymentNotModifiable},
		{name: "a missing reference", refunds: requested(models.RefundStatusRequested), reference: " ", wantErr: apperrors.ErrInvalidPaymentInput},
		{name: "a reference that is too long", refunds: requested(models.RefundStatusRequested), reference: strings.Repeat("9", constant.MaxRefundReferenceLength+1), wantErr: apperrors.ErrInvalidPaymentInput},
		{name: "an unknown refund", reference: "412345678901", wantErr: apperrors.ErrRefundNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appointments := &fakeAppointmentRepo{}
			payments := &fakePaymentRepo{payment: &models.Payment{ID: 70, AppointmentID: 50, Amount: 40000, Status: models.PaymentStatusCaptured}, refunds: tt.refunds}
			outbox := &fakeOutboxRepo{}
			_, service := newTestAppointmentAndPaymentServices(t, &fakeDoctorRepo{}, appointments, outbox, &fakeUserPatientRepo{}, payments, &fakeGateway{}, time.Now())

			completed, err := service.CompleteRefund(context.Background(), 1, 90, tt.reference)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("CompleteRefund error = %v, want %v", err, tt.wantErr)
				}
				if payments.payment.Status != models.PaymentStatusCaptured || len(outbox.events) != 0 {
					t.Fatalf("CompleteRefund left %q with outbox %+v after an error", payments.payment.Status, outbox.events)
				}
				return
			}
			if err != nil {
				t.Fatalf("CompleteRefund returned error: %v", err)
			}
			if completed.Status != models.RefundStatusSucceeded || completed.ProviderRefundID != "412345678901" {
				t.Fatalf("CompleteRefund = %+v, want succeeded with the trimmed reference", completed)
			}
			if payments.payment.Status != models.PaymentStatusRefunded || appointments.paymentStatusTo != models.AppointmentPaymentRefunded {
				t.Fatalf("CompleteRefund left the payment %q and the appointment %q, want refunded", payments.payment.Status, appointments.paymentStatusTo)
			}
			if len(outbox.events) != 1 || outbox.events[0].EventType != models.OutboxEventPaymentRefunded {
				t.Fatalf("CompleteRefund outbox = %+v, want one refunded event", outbox.events)
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
		{name: "no status lists the manual queue", wantStatus: models.RefundStatusRequested},
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
