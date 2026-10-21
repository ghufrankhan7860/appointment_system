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

func unpaidAt(startsAt time.Time, dueAt time.Time) *models.AppointmentDetails {
	appointment := scheduledAt(startsAt, models.AppointmentStatusScheduled)
	appointment.PaymentStatus = models.AppointmentPaymentUnpaid
	appointment.PaymentDueAt = &dueAt
	return appointment
}

func TestAppointmentServiceBookSetsPaymentHold(t *testing.T) {
	location := clinicLocation(t)
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, location)
	nineAM := time.Date(2026, 10, 26, 9, 0, 0, 0, location)

	tests := []struct {
		name       string
		fee        int
		wantStatus models.AppointmentPaymentStatus
		wantDue    bool
	}{
		{name: "a fee makes the booking unpaid with a hold", fee: 50000, wantStatus: models.AppointmentPaymentUnpaid, wantDue: true},
		{name: "a free consultation is waived", fee: 0, wantStatus: models.AppointmentPaymentWaived},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doctor := clinicDoctor(models.VerificationStatusVerified)
			doctor.ConsultationFee = tt.fee
			appointments := &fakeAppointmentRepo{}
			service := newTestAppointmentService(t, &fakeDoctorRepo{byID: doctor}, appointments, &fakeOutboxRepo{}, &fakeUserPatientRepo{activeResult: patientLink(models.AccessLevelFull)}, now)

			if _, err := service.Book(context.Background(), 7, &models.Appointment{PatientID: 3, DoctorID: 5, StartsAt: nineAM, Mode: models.ConsultationModeInPerson}); err != nil {
				t.Fatalf("Book returned error: %v", err)
			}
			created := appointments.createInput
			if created.PaymentStatus != tt.wantStatus {
				t.Fatalf("Book payment status = %q, want %q", created.PaymentStatus, tt.wantStatus)
			}
			if tt.wantDue && (created.PaymentDueAt == nil || !created.PaymentDueAt.Equal(now.Add(constant.PaymentHold))) {
				t.Fatalf("Book payment due = %v, want %v", created.PaymentDueAt, now.Add(constant.PaymentHold))
			}
			if !tt.wantDue && created.PaymentDueAt != nil {
				t.Fatalf("Book payment due = %v, want none for a free consultation", created.PaymentDueAt)
			}
		})
	}
}

func TestPaymentServiceCreate(t *testing.T) {
	location := clinicLocation(t)
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, location)
	nineAM := time.Date(2026, 10, 26, 9, 0, 0, 0, location)
	withStatus := func(status models.AppointmentPaymentStatus) *models.AppointmentDetails {
		appointment := unpaidAt(nineAM, now.Add(5*time.Minute))
		appointment.PaymentStatus = status
		return appointment
	}
	expired := unpaidAt(nineAM, now.Add(-time.Minute))
	cancelled := unpaidAt(nineAM, now.Add(5*time.Minute))
	cancelled.Status = models.AppointmentStatusCancelled

	tests := []struct {
		name        string
		link        *models.UserPatient
		appointment *models.AppointmentDetails
		existing    *models.Payment
		wantErr     error
	}{
		{name: "full access opens a payment", link: patientLink(models.AccessLevelFull), appointment: unpaidAt(nineAM, now.Add(5*time.Minute))},
		{name: "a caregiver with appointments_only access can pay", link: patientLink(models.AccessLevelAppointmentsOnly), appointment: unpaidAt(nineAM, now.Add(5*time.Minute))},
		{name: "asking again returns the live payment", link: patientLink(models.AccessLevelFull), appointment: unpaidAt(nineAM, now.Add(5*time.Minute)), existing: &models.Payment{ID: 70, AppointmentID: 50, Status: models.PaymentStatusCreated, ProviderOrderID: "order-old"}},
		{name: "read_only access cannot pay", link: patientLink(models.AccessLevelReadOnly), appointment: unpaidAt(nineAM, now.Add(5*time.Minute)), wantErr: apperrors.ErrPatientForbidden},
		{name: "user with no link", appointment: unpaidAt(nineAM, now.Add(5*time.Minute)), wantErr: apperrors.ErrAppointmentNotFound},
		{name: "already paid", link: patientLink(models.AccessLevelFull), appointment: withStatus(models.AppointmentPaymentPaid), wantErr: apperrors.ErrPaymentNotRequired},
		{name: "free consultation", link: patientLink(models.AccessLevelFull), appointment: withStatus(models.AppointmentPaymentWaived), wantErr: apperrors.ErrPaymentNotRequired},
		{name: "hold expired", link: patientLink(models.AccessLevelFull), appointment: expired, wantErr: apperrors.ErrPaymentWindowClosed},
		{name: "booking cancelled", link: patientLink(models.AccessLevelFull), appointment: cancelled, wantErr: apperrors.ErrPaymentWindowClosed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userPatients := &fakeUserPatientRepo{activeResult: tt.link}
			if tt.link == nil {
				userPatients.activeErr = apperrors.ErrUserPatientNotFound
			}
			payments := &fakePaymentRepo{payment: tt.existing}
			gateway := &fakeGateway{}
			_, service := newTestAppointmentAndPaymentServices(t, &fakeDoctorRepo{}, &fakeAppointmentRepo{byID: tt.appointment}, &fakeOutboxRepo{}, userPatients, payments, gateway, now)

			payment, payload, err := service.Create(context.Background(), 7, 50)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Create error = %v, want %v", err, tt.wantErr)
				}
				if gateway.orders != 0 {
					t.Fatalf("Create opened %d gateway orders after an error", gateway.orders)
				}
				return
			}
			if err != nil {
				t.Fatalf("Create returned error: %v", err)
			}
			if tt.existing != nil {
				if gateway.orders != 0 || payload["orderId"] != "order-old" {
					t.Fatalf("Create opened %d orders with payload %v, want the existing order", gateway.orders, payload)
				}
				return
			}
			if payment.Amount != 40000 || payment.Currency != constant.PaymentCurrency || payment.PayerUserID != 7 || payment.Status != models.PaymentStatusCreated || payment.Provider != "fake" {
				t.Fatalf("Create payment = %+v, want 40000 %s created by 7 through fake", payment, constant.PaymentCurrency)
			}
			if payload["orderId"] != "order-1" || len(payments.changes) != 1 {
				t.Fatalf("Create payload %v with %d history rows, want order-1 and one row", payload, len(payments.changes))
			}
		})
	}
}

func TestPaymentServiceVerify(t *testing.T) {
	location := clinicLocation(t)
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, location)
	nineAM := time.Date(2026, 10, 26, 9, 0, 0, 0, location)
	openPayment := func() *models.Payment {
		return &models.Payment{ID: 70, AppointmentID: 50, PayerUserID: 7, Amount: 40000, Currency: constant.PaymentCurrency, Status: models.PaymentStatusCreated, ProviderOrderID: "order-1"}
	}
	capturedPayment := openPayment()
	capturedPayment.Status = models.PaymentStatusCaptured
	capturedPayment.ProviderPaymentID = "pay-1"

	tests := []struct {
		name          string
		appointment   *models.AppointmentDetails
		payment       *models.Payment
		verifyErr     error
		markPaidErr   error
		updateErr     error
		paymentID     string
		wantErr       error
		wantStatus    models.PaymentStatus
		wantRefunded  int
		wantMarkPaid  bool
		wantOutboxEvt string
	}{
		{name: "a verified payment captures and marks the booking paid", appointment: unpaidAt(nineAM, now.Add(5*time.Minute)), payment: openPayment(), wantStatus: models.PaymentStatusCaptured, wantMarkPaid: true, wantOutboxEvt: models.OutboxEventPaymentCaptured},
		{name: "a bad signature changes nothing", appointment: unpaidAt(nineAM, now.Add(5*time.Minute)), payment: openPayment(), verifyErr: apperrors.ErrPaymentVerificationFailed, wantErr: apperrors.ErrPaymentVerificationFailed, wantStatus: models.PaymentStatusCreated},
		{name: "verifying a captured payment again returns it", appointment: unpaidAt(nineAM, now.Add(5*time.Minute)), payment: capturedPayment, wantStatus: models.PaymentStatusCaptured},
		{name: "payment after the hold is refunded", appointment: unpaidAt(nineAM, now.Add(-time.Minute)), payment: openPayment(), wantErr: apperrors.ErrPaymentWindowClosed, wantStatus: models.PaymentStatusRefunded, wantRefunded: 40000, wantOutboxEvt: models.OutboxEventPaymentRefunded},
		{name: "payment that loses the race to the expiry is refunded", appointment: unpaidAt(nineAM, now.Add(5*time.Minute)), payment: openPayment(), markPaidErr: apperrors.ErrAppointmentNotModifiable, wantErr: apperrors.ErrPaymentWindowClosed, wantStatus: models.PaymentStatusRefunded, wantRefunded: 40000, wantMarkPaid: true, wantOutboxEvt: models.OutboxEventPaymentRefunded},
		{name: "a gateway payment id already used elsewhere", appointment: unpaidAt(nineAM, now.Add(5*time.Minute)), payment: openPayment(), updateErr: apperrors.ErrProviderPaymentUsed, wantErr: apperrors.ErrPaymentVerificationFailed, wantStatus: models.PaymentStatusCreated},
		{name: "missing provider payment id", appointment: unpaidAt(nineAM, now.Add(5*time.Minute)), payment: openPayment(), paymentID: " ", wantErr: apperrors.ErrInvalidPaymentInput, wantStatus: models.PaymentStatusCreated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.paymentID == "" {
				tt.paymentID = "pay-1"
			}
			appointments := &fakeAppointmentRepo{byID: tt.appointment, markPaidErr: tt.markPaidErr}
			payments := &fakePaymentRepo{payment: tt.payment, updateErr: tt.updateErr}
			gateway := &fakeGateway{verifyErr: tt.verifyErr}
			outbox := &fakeOutboxRepo{}
			_, service := newTestAppointmentAndPaymentServices(t, &fakeDoctorRepo{}, appointments, outbox, &fakeUserPatientRepo{activeResult: patientLink(models.AccessLevelAppointmentsOnly)}, payments, gateway, now)

			_, err := service.Verify(context.Background(), 7, 70, tt.paymentID, "signature")
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("Verify error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && err != nil {
				t.Fatalf("Verify returned error: %v", err)
			}
			if payments.payment.Status != tt.wantStatus {
				t.Fatalf("Verify left the payment %q, want %q", payments.payment.Status, tt.wantStatus)
			}
			if gateway.refunded != tt.wantRefunded || appointments.markPaidCalled != tt.wantMarkPaid {
				t.Fatalf("Verify refunded %d and marked paid %v, want %d and %v", gateway.refunded, appointments.markPaidCalled, tt.wantRefunded, tt.wantMarkPaid)
			}
			if tt.wantOutboxEvt == "" && len(outbox.events) != 0 || tt.wantOutboxEvt != "" && (len(outbox.events) != 1 || outbox.events[0].EventType != tt.wantOutboxEvt) {
				t.Fatalf("Verify outbox = %+v, want %q", outbox.events, tt.wantOutboxEvt)
			}
		})
	}
}

func TestPaymentServiceRefund(t *testing.T) {
	errGatewayDown := errors.New("gateway down")
	payment := func(status models.PaymentStatus) *models.Payment {
		return &models.Payment{ID: 70, AppointmentID: 50, Amount: 40000, Status: status}
	}

	tests := []struct {
		name        string
		payment     *models.Payment
		refundErr   error
		wantErr     error
		wantStatus  models.PaymentStatus
		wantRefunds []models.RefundStatus
	}{
		{name: "admin refunds a captured payment in full", payment: payment(models.PaymentStatusCaptured), wantStatus: models.PaymentStatusRefunded, wantRefunds: []models.RefundStatus{models.RefundStatusSucceeded}},
		{name: "a payment that was never captured", payment: payment(models.PaymentStatusCreated), wantErr: apperrors.ErrPaymentNotModifiable, wantStatus: models.PaymentStatusCreated},
		{name: "a gateway failure is recorded", payment: payment(models.PaymentStatusCaptured), refundErr: errGatewayDown, wantErr: errGatewayDown, wantStatus: models.PaymentStatusCaptured, wantRefunds: []models.RefundStatus{models.RefundStatusFailed}},
		{name: "missing payment", wantErr: apperrors.ErrPaymentNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appointments := &fakeAppointmentRepo{}
			payments := &fakePaymentRepo{payment: tt.payment}
			outbox := &fakeOutboxRepo{}
			_, service := newTestAppointmentAndPaymentServices(t, &fakeDoctorRepo{}, appointments, outbox, &fakeUserPatientRepo{}, payments, &fakeGateway{refundErr: tt.refundErr}, time.Now())

			_, err := service.Refund(context.Background(), 1, 70, "goodwill")
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("Refund error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && err != nil {
				t.Fatalf("Refund returned error: %v", err)
			}
			if tt.payment == nil {
				return
			}
			if payments.payment.Status != tt.wantStatus || len(payments.finished) != len(tt.wantRefunds) {
				t.Fatalf("Refund left %q with refunds %v, want %q and %v", payments.payment.Status, payments.finished, tt.wantStatus, tt.wantRefunds)
			}
			for index, status := range tt.wantRefunds {
				if payments.finished[index] != status {
					t.Fatalf("Refund finished %v, want %v", payments.finished, tt.wantRefunds)
				}
			}
			if tt.wantStatus == models.PaymentStatusRefunded {
				if payments.refunds[0].Reason != models.RefundReasonAdmin || payments.refunds[0].InitiatedByUserID != 1 || payments.refunds[0].Amount != 40000 {
					t.Fatalf("Refund row = %+v, want a full admin refund by 1", payments.refunds[0])
				}
				if appointments.paymentStatusTo != models.AppointmentPaymentRefunded || len(outbox.events) != 1 || outbox.events[0].EventType != models.OutboxEventPaymentRefunded {
					t.Fatalf("Refund set the appointment %q with outbox %+v, want refunded and one refunded event", appointments.paymentStatusTo, outbox.events)
				}
			}
		})
	}
}

func TestPaymentServiceExpireUnpaid(t *testing.T) {
	location := clinicLocation(t)
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, location)
	nineAM := time.Date(2026, 10, 26, 9, 0, 0, 0, location)

	t.Run("an unpaid booking past its hold is cancelled and its order expired", func(t *testing.T) {
		appointments := &fakeAppointmentRepo{unpaidDue: []*models.AppointmentDetails{unpaidAt(nineAM, now.Add(-time.Minute))}}
		payments := &fakePaymentRepo{payment: &models.Payment{ID: 70, AppointmentID: 50, Status: models.PaymentStatusCreated}}
		outbox := &fakeOutboxRepo{}
		_, service := newTestAppointmentAndPaymentServices(t, &fakeDoctorRepo{}, appointments, outbox, &fakeUserPatientRepo{}, payments, &fakeGateway{}, now)

		expired, err := service.ExpireUnpaid(context.Background(), 10)
		if err != nil || expired != 1 {
			t.Fatalf("ExpireUnpaid = %d, %v; want 1", expired, err)
		}
		change := appointments.statusChanges[0]
		if change.ToStatus != models.AppointmentStatusCancelled || change.ChangedByUserID != 0 {
			t.Fatalf("ExpireUnpaid status change = %+v, want a system cancel", change)
		}
		if len(outbox.events) != 1 || outbox.events[0].Payload["cancelReason"] != models.CancelReasonPaymentExpired {
			t.Fatalf("ExpireUnpaid outbox = %+v, want one cancelled event with reason payment_expired", outbox.events)
		}
		if payments.payment.Status != models.PaymentStatusExpired {
			t.Fatalf("ExpireUnpaid left the order %q, want expired", payments.payment.Status)
		}
	})

	t.Run("a booking paid meanwhile is skipped", func(t *testing.T) {
		appointments := &fakeAppointmentRepo{unpaidDue: []*models.AppointmentDetails{unpaidAt(nineAM, now.Add(-time.Minute))}, cancelUnpaidErr: apperrors.ErrAppointmentNotModifiable}
		outbox := &fakeOutboxRepo{}
		_, service := newTestAppointmentAndPaymentServices(t, &fakeDoctorRepo{}, appointments, outbox, &fakeUserPatientRepo{}, &fakePaymentRepo{}, &fakeGateway{}, now)

		expired, err := service.ExpireUnpaid(context.Background(), 10)
		if err != nil || expired != 0 || len(outbox.events) != 0 {
			t.Fatalf("ExpireUnpaid = %d, %v with %d events; want nothing changed", expired, err, len(outbox.events))
		}
	})
}

func TestAppointmentServicePaymentRules(t *testing.T) {
	location := clinicLocation(t)
	nineAM := time.Date(2026, 10, 26, 9, 0, 0, 0, location)
	dayBefore := nineAM.Add(-21 * time.Hour)
	doctor := clinicDoctor(models.VerificationStatusVerified)

	t.Run("cancelling a paid booking refunds it in full", func(t *testing.T) {
		for _, side := range []struct {
			role       models.Role
			link       *models.UserPatient
			wantReason models.RefundReason
		}{
			{role: models.RolePatient, link: patientLink(models.AccessLevelFull), wantReason: models.RefundReasonPatientCancelled},
			{role: models.RoleDoctor, wantReason: models.RefundReasonDoctorCancelled},
		} {
			payments := &fakePaymentRepo{payment: &models.Payment{ID: 70, AppointmentID: 50, Amount: 40000, Status: models.PaymentStatusCaptured}}
			gateway := &fakeGateway{}
			userPatients := &fakeUserPatientRepo{activeResult: side.link}
			if side.link == nil {
				userPatients.activeErr = apperrors.ErrUserPatientNotFound
			}
			service, _ := newTestAppointmentAndPaymentServices(t, &fakeDoctorRepo{byUserID: doctor}, &fakeAppointmentRepo{byID: scheduledAt(nineAM, models.AppointmentStatusScheduled)}, &fakeOutboxRepo{}, userPatients, payments, gateway, dayBefore)

			if _, err := service.Cancel(context.Background(), 11, side.role, 50, ""); err != nil {
				t.Fatalf("Cancel as %s returned error: %v", side.role, err)
			}
			if gateway.refunded != 40000 || payments.refunds[0].Reason != side.wantReason {
				t.Fatalf("Cancel as %s refunded %d for %q, want 40000 for %q", side.role, gateway.refunded, payments.refunds[0].Reason, side.wantReason)
			}
		}
	})

	t.Run("cancelling an unpaid booking expires its open order", func(t *testing.T) {
		payments := &fakePaymentRepo{payment: &models.Payment{ID: 70, AppointmentID: 50, Status: models.PaymentStatusCreated}}
		gateway := &fakeGateway{}
		service, _ := newTestAppointmentAndPaymentServices(t, &fakeDoctorRepo{}, &fakeAppointmentRepo{byID: unpaidAt(nineAM, dayBefore.Add(5*time.Minute))}, &fakeOutboxRepo{}, &fakeUserPatientRepo{activeResult: patientLink(models.AccessLevelFull)}, payments, gateway, dayBefore)

		if _, err := service.Cancel(context.Background(), 7, models.RolePatient, 50, ""); err != nil {
			t.Fatalf("Cancel returned error: %v", err)
		}
		if payments.payment.Status != models.PaymentStatusExpired || gateway.refunded != 0 {
			t.Fatalf("Cancel left the order %q and refunded %d, want expired and nothing refunded", payments.payment.Status, gateway.refunded)
		}
	})

	t.Run("an unpaid appointment cannot be completed", func(t *testing.T) {
		service := newTestAppointmentService(t, &fakeDoctorRepo{byUserID: doctor}, &fakeAppointmentRepo{byID: unpaidAt(nineAM, nineAM.Add(-time.Hour))}, &fakeOutboxRepo{}, &fakeUserPatientRepo{activeErr: apperrors.ErrUserPatientNotFound}, nineAM.Add(10*time.Minute))

		if _, err := service.Complete(context.Background(), 11, models.RoleDoctor, 50); !errors.Is(err, apperrors.ErrPaymentPending) {
			t.Fatalf("Complete error = %v, want %v", err, apperrors.ErrPaymentPending)
		}
	})

	t.Run("rescheduling carries the payment to the new appointment", func(t *testing.T) {
		current := scheduledAt(nineAM, models.AppointmentStatusScheduled)
		current.ID = 40
		appointments := &fakeAppointmentRepo{byID: current, booked: []*models.AppointmentDetails{current}}
		payments := &fakePaymentRepo{}
		service, _ := newTestAppointmentAndPaymentServices(t, &fakeDoctorRepo{byID: doctor}, appointments, &fakeOutboxRepo{}, &fakeUserPatientRepo{activeResult: patientLink(models.AccessLevelFull)}, payments, &fakeGateway{}, dayBefore)

		if _, err := service.Reschedule(context.Background(), 7, models.RolePatient, 40, nineAM.Add(time.Hour), "", ""); err != nil {
			t.Fatalf("Reschedule returned error: %v", err)
		}
		if appointments.createInput.PaymentStatus != models.AppointmentPaymentPaid || payments.movedFrom != 40 || payments.movedTo != 50 {
			t.Fatalf("Reschedule payment %q moved %d -> %d, want paid moved 40 -> 50", appointments.createInput.PaymentStatus, payments.movedFrom, payments.movedTo)
		}
	})
}
