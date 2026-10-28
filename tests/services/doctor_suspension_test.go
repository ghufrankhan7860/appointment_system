package services_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
)

func TestDoctorVerificationServiceSuspendCascade(t *testing.T) {
	location := clinicLocation(t)
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, location)
	booking := func(id uint64, startsAt time.Time, status models.AppointmentStatus, payment models.AppointmentPaymentStatus) *models.AppointmentDetails {
		appointment := scheduledAt(startsAt, status)
		appointment.ID = id
		appointment.PaymentStatus = payment
		return appointment
	}
	upcoming := func() []*models.AppointmentDetails {
		original := uint64(53)
		placeholder := booking(60, now.Add(50*time.Hour), models.AppointmentStatusRescheduleProposed, models.AppointmentPaymentPaid)
		placeholder.RescheduledFromID = &original
		return []*models.AppointmentDetails{
			booking(54, now.Add(-10*time.Minute), models.AppointmentStatusScheduled, models.AppointmentPaymentPaid),
			booking(51, now.Add(24*time.Hour), models.AppointmentStatusScheduled, models.AppointmentPaymentPaid),
			booking(52, now.Add(26*time.Hour), models.AppointmentStatusScheduled, models.AppointmentPaymentUnpaid),
			booking(53, now.Add(48*time.Hour), models.AppointmentStatusRescheduleProposed, models.AppointmentPaymentPaid),
			placeholder,
			booking(56, now.Add(72*time.Hour), models.AppointmentStatusCancelled, models.AppointmentPaymentRefunded),
		}
	}
	newSuspension := func(appointments *fakeAppointmentRepo, payments *fakePaymentRepo, gateway *fakeGateway, outbox *fakeOutboxRepo) (*services.DoctorVerificationService, *fakeDoctorRepo) {
		doctors := &fakeDoctorRepo{byID: clinicDoctor(models.VerificationStatusVerified)}
		appointmentService, _ := newTestAppointmentAndPaymentServices(t, doctors, appointments, outbox, &fakeUserPatientRepo{}, payments, gateway, now)
		service, err := services.NewDoctorVerificationService(doctors, &fakeDoctorVerificationRepo{}, appointmentService, &fakeTxManager{})
		if err != nil {
			t.Fatalf("NewDoctorVerificationService returned error: %v", err)
		}
		return service, doctors
	}
	paidPayment := func() *models.Payment {
		return &models.Payment{ID: 70, AppointmentID: 51, Amount: 40000, Status: models.PaymentStatusCaptured}
	}
	pending := &models.RescheduleProposal{ID: 80, AppointmentID: 53, PlaceholderAppointmentID: 60, Status: models.ProposalStatusPending}

	t.Run("bookings that have not started are cancelled and paid ones refunded", func(t *testing.T) {
		appointments := &fakeAppointmentRepo{booked: upcoming(), proposal: pending}
		payments := &fakePaymentRepo{payment: paidPayment()}
		gateway := &fakeGateway{}
		outbox := &fakeOutboxRepo{}
		service, doctors := newSuspension(appointments, payments, gateway, outbox)

		suspended, result, err := service.Suspend(context.Background(), 1, 5, "complaint upheld")
		if err != nil {
			t.Fatalf("Suspend returned error: %v", err)
		}
		if suspended.VerificationStatus != models.VerificationStatusSuspended || doctors.updateStatusTo != models.VerificationStatusSuspended {
			t.Fatalf("Suspend left the doctor %q, want suspended", suspended.VerificationStatus)
		}
		if result.CancelledAppointments != 3 || result.RefundsRequested != 2 {
			t.Fatalf("Suspend result = %+v, want 3 cancelled and 2 refunds", result)
		}
		if !slices.Equal(appointments.cancelledIDs, []uint64{51, 52, 60, 53}) {
			t.Fatalf("Suspend cancelled %v, want 51, 52, the placeholder 60 and its original 53", appointments.cancelledIDs)
		}
		if appointments.cancelReason != models.CancelReasonDoctorSuspended || appointments.closedTo != models.ProposalStatusCancelled || appointments.closedBy != 1 {
			t.Fatalf("Suspend cancel reason %q, proposal closed %q by %d; want doctor_suspended and cancelled by 1", appointments.cancelReason, appointments.closedTo, appointments.closedBy)
		}
		fromStatus := map[uint64]models.AppointmentStatus{}
		for _, change := range appointments.statusChanges {
			fromStatus[change.AppointmentID] = change.FromStatus
		}
		if fromStatus[51] != models.AppointmentStatusScheduled || fromStatus[53] != models.AppointmentStatusRescheduleProposed || fromStatus[60] != models.AppointmentStatusRescheduleProposed {
			t.Fatalf("Suspend history from-statuses = %v", fromStatus)
		}
		cancelEvents := 0
		for _, event := range outbox.events {
			if event.EventType == models.OutboxEventAppointmentCancelled {
				cancelEvents++
				if event.Payload["cancelReason"] != models.CancelReasonDoctorSuspended {
					t.Fatalf("cancel event reason = %v, want doctor_suspended", event.Payload["cancelReason"])
				}
			}
		}
		if cancelEvents != 3 {
			t.Fatalf("Suspend wrote %d cancel events, want 3", cancelEvents)
		}
		if gateway.refunded != 40000 || payments.refunds[0].Reason != models.RefundReasonDoctorCancelled || payments.payment.Status != models.PaymentStatusRefunded {
			t.Fatalf("Suspend refunded %d with %+v, want the full fee as doctor_cancelled", gateway.refunded, payments.refunds)
		}
	})

	t.Run("a doctor with no upcoming bookings", func(t *testing.T) {
		service, _ := newSuspension(&fakeAppointmentRepo{}, &fakePaymentRepo{}, &fakeGateway{}, &fakeOutboxRepo{})

		_, result, err := service.Suspend(context.Background(), 1, 5, "complaint upheld")
		if err != nil || result.CancelledAppointments != 0 || result.RefundsRequested != 0 {
			t.Fatalf("Suspend = %+v, %v; want nothing cancelled", result, err)
		}
	})

	t.Run("a refund failure stops with an error after the cancellations", func(t *testing.T) {
		errGatewayDown := errors.New("gateway down")
		appointments := &fakeAppointmentRepo{booked: upcoming(), proposal: pending}
		payments := &fakePaymentRepo{payment: paidPayment()}
		service, doctors := newSuspension(appointments, payments, &fakeGateway{refundErr: errGatewayDown}, &fakeOutboxRepo{})

		_, _, err := service.Suspend(context.Background(), 1, 5, "complaint upheld")
		if !errors.Is(err, errGatewayDown) {
			t.Fatalf("Suspend error = %v, want %v", err, errGatewayDown)
		}
		if doctors.updateStatusTo != models.VerificationStatusSuspended || len(appointments.cancelledIDs) != 4 {
			t.Fatalf("Suspend left the doctor %q with %v cancelled, want suspended and all four cancelled", doctors.updateStatusTo, appointments.cancelledIDs)
		}
		if len(payments.finished) != 1 || payments.finished[0] != models.RefundStatusFailed {
			t.Fatalf("Suspend finished refunds %v, want one failed refund", payments.finished)
		}
	})
}
