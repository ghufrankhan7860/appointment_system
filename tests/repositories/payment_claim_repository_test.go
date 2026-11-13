package repositories_test

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/repositories"
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"
)

func TestPaymentRepositoryClaimsAndManualRefunds(t *testing.T) {
	db := openTestDB(t)
	fixture := newAppointmentFixture(t, db)
	ctx := context.Background()
	payments, err := repositories.NewPaymentRepository(db)
	if err != nil {
		t.Fatalf("NewPaymentRepository returned error: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM refunds WHERE payment_id IN (SELECT id FROM payments WHERE payer_user_id = ?)", fixture.booker.ID)
		db.Exec("DELETE FROM payment_status_history WHERE payment_id IN (SELECT id FROM payments WHERE payer_user_id = ?)", fixture.booker.ID)
		db.Exec("DELETE FROM payments WHERE payer_user_id = ?", fixture.booker.ID)
	})

	startsAt := time.Now().Add(160 * time.Hour).Truncate(time.Hour)
	appointment := newUnpaidAppointment(t, fixture, startsAt, time.Now().Add(10*time.Minute))
	openUPI := func(appointmentID uint64) *models.Payment {
		t.Helper()
		created, err := payments.Create(ctx, &models.Payment{
			AppointmentID:   appointmentID,
			PayerUserID:     fixture.booker.ID,
			Amount:          50000,
			Currency:        "INR",
			Provider:        models.ProviderUPIDirect,
			ProviderOrderID: fmt.Sprintf("upi_direct_%d_%d", appointmentID, time.Now().UnixNano()),
			Status:          models.PaymentStatusCreated,
		})
		if err != nil {
			t.Fatalf("Create upi payment returned error: %v", err)
		}
		return created
	}
	listed := func(found []*models.Payment, id uint64) bool {
		return slices.ContainsFunc(found, func(payment *models.Payment) bool { return payment.ID == id })
	}

	payment := openUPI(appointment.ID)
	proofKey := "payments/proof-test"
	claimedAt := time.Now()
	if err := payments.Claim(ctx, payment.ID, "412356789012", &proofKey, claimedAt); err != nil {
		t.Fatalf("Claim returned error: %v", err)
	}
	if err := payments.Claim(ctx, payment.ID, "412356789012", nil, claimedAt); !errors.Is(err, apperrors.ErrPaymentNotModifiable) {
		t.Fatalf("Claim twice error = %v, want %v", err, apperrors.ErrPaymentNotModifiable)
	}
	claimed, err := payments.GetByID(ctx, payment.ID)
	if err != nil || claimed.Status != models.PaymentStatusClaimed || claimed.PayerReference != "412356789012" || claimed.ProofStorageKey == nil || *claimed.ProofStorageKey != proofKey || claimed.ClaimedAt == nil {
		t.Fatalf("GetByID after the claim = %+v, %v; want claimed with its reference, proof and time", claimed, err)
	}
	if active, err := payments.GetActiveByAppointment(ctx, appointment.ID); err != nil || active.ID != payment.ID {
		t.Fatalf("GetActiveByAppointment = %+v, %v; want the claimed payment", active, err)
	}
	if _, err := payments.Create(ctx, &models.Payment{AppointmentID: appointment.ID, PayerUserID: fixture.booker.ID, Amount: 50000, Currency: "INR", Provider: "none", ProviderOrderID: fmt.Sprintf("order-x-%d", time.Now().UnixNano()), Status: models.PaymentStatusCreated}); !errors.Is(err, apperrors.ErrPaymentAlreadyExists) {
		t.Fatalf("Create beside a claimed payment error = %v, want %v", err, apperrors.ErrPaymentAlreadyExists)
	}

	if found, err := payments.ListClaimed(ctx, fixture.doctor.ID, 50, 0); err != nil || !listed(found, payment.ID) {
		t.Fatalf("ListClaimed for the doctor = %d rows, %v; want the claim", len(found), err)
	}
	if found, err := payments.ListClaimed(ctx, 0, 200, 0); err != nil || !listed(found, payment.ID) {
		t.Fatalf("ListClaimed for every doctor = %d rows, %v; want the claim", len(found), err)
	}
	if found, err := payments.ListClaimed(ctx, fixture.doctor.ID+1_000_000, 50, 0); err != nil || listed(found, payment.ID) {
		t.Fatalf("ListClaimed for another doctor = %d rows, %v; want not the claim", len(found), err)
	}

	if err := payments.Review(ctx, payment.ID, models.PaymentStatusCaptured, fixture.booker.ID, "seen", time.Now()); err != nil {
		t.Fatalf("Review returned error: %v", err)
	}
	if err := payments.Review(ctx, payment.ID, models.PaymentStatusRejected, fixture.booker.ID, "", time.Now()); !errors.Is(err, apperrors.ErrPaymentNotModifiable) {
		t.Fatalf("Review twice error = %v, want %v", err, apperrors.ErrPaymentNotModifiable)
	}
	reviewed, err := payments.GetByID(ctx, payment.ID)
	if err != nil || reviewed.Status != models.PaymentStatusCaptured || reviewed.CapturedAt == nil || reviewed.ReviewedAt == nil || reviewed.ReviewedByUserID != fixture.booker.ID || reviewed.ReviewNote != "seen" {
		t.Fatalf("GetByID after the review = %+v, %v; want captured and reviewed with its note", reviewed, err)
	}
	if found, err := payments.ListClaimed(ctx, fixture.doctor.ID, 50, 0); err != nil || listed(found, payment.ID) {
		t.Fatalf("ListClaimed after the review = %d rows, %v; want the claim gone", len(found), err)
	}

	refund, err := payments.CreateRefund(ctx, &models.Refund{PaymentID: payment.ID, Amount: 50000, Reason: models.RefundReasonPatientCancelled, Status: models.RefundStatusRequested})
	if err != nil {
		t.Fatalf("CreateRefund returned error: %v", err)
	}
	refundListed := func(found []*models.Refund) bool {
		return slices.ContainsFunc(found, func(item *models.Refund) bool { return item.ID == refund.ID })
	}
	if found, err := payments.ListManualRefunds(ctx, fixture.doctor.ID, models.RefundStatusRequested, 50, 0); err != nil || !refundListed(found) {
		t.Fatalf("ListManualRefunds for the doctor = %d rows, %v; want the refund", len(found), err)
	}
	if found, err := payments.ListManualRefunds(ctx, fixture.doctor.ID+1_000_000, models.RefundStatusRequested, 50, 0); err != nil || refundListed(found) {
		t.Fatalf("ListManualRefunds for another doctor = %d rows, %v; want not the refund", len(found), err)
	}

	collected, err := payments.Create(ctx, &models.Payment{AppointmentID: newUnpaidAppointment(t, fixture, startsAt.Add(time.Hour), time.Now().Add(10*time.Minute)).ID, PayerUserID: fixture.booker.ID, Amount: 50000, Currency: "INR", Provider: models.ProviderClinic, ProviderOrderID: fmt.Sprintf("clinic-%d", time.Now().UnixNano()), Status: models.PaymentStatusCaptured, CapturedAt: &claimedAt, ReviewedByUserID: fixture.booker.ID, ReviewedAt: &claimedAt, ReviewNote: "cash"})
	if err != nil || collected.CapturedAt == nil || collected.ReviewNote != "cash" {
		t.Fatalf("Create clinic payment = %+v, %v; want it captured with its note", collected, err)
	}
	if _, err := payments.Create(ctx, &models.Payment{AppointmentID: appointment.ID, PayerUserID: fixture.booker.ID, Amount: 50000, Currency: "INR", Provider: "cash", ProviderOrderID: fmt.Sprintf("cash-%d", time.Now().UnixNano()), Status: models.PaymentStatusExpired}); err == nil {
		t.Fatal("Create with an unknown provider succeeded, want the provider check to refuse it")
	}
}

func TestAppointmentRepositoryManualPaymentStates(t *testing.T) {
	db := openTestDB(t)
	fixture := newAppointmentFixture(t, db)
	ctx := context.Background()
	startsAt := time.Now().Add(170 * time.Hour).Truncate(time.Hour)

	pending := func(offset time.Duration) *models.Appointment {
		t.Helper()
		appointment := newUnpaidAppointment(t, fixture, startsAt.Add(offset), time.Now().Add(10*time.Minute))
		dueAt := appointment.StartsAt
		if err := fixture.appointments.SetPaymentDue(ctx, appointment.ID, models.AppointmentPaymentUnpaid, models.AppointmentPaymentPendingVerification, &dueAt); err != nil {
			t.Fatalf("SetPaymentDue returned error: %v", err)
		}
		return appointment
	}

	t.Run("a claim holds the slot until the start", func(t *testing.T) {
		appointment := pending(0)
		if err := fixture.appointments.SetPaymentDue(ctx, appointment.ID, models.AppointmentPaymentUnpaid, models.AppointmentPaymentPendingVerification, nil); !errors.Is(err, apperrors.ErrAppointmentNotModifiable) {
			t.Fatalf("SetPaymentDue from the wrong status error = %v, want %v", err, apperrors.ErrAppointmentNotModifiable)
		}
		found, err := fixture.appointments.GetByID(ctx, appointment.ID)
		if err != nil || found.PaymentStatus != models.AppointmentPaymentPendingVerification || found.PaymentDueAt == nil || !found.PaymentDueAt.Equal(appointment.StartsAt) {
			t.Fatalf("GetByID = %+v, %v; want pending_verification due at the start", found, err)
		}

		if err := fixture.appointments.SetPaymentDue(ctx, appointment.ID, models.AppointmentPaymentPendingVerification, models.AppointmentPaymentPaid, nil); err != nil {
			t.Fatalf("SetPaymentDue to paid returned error: %v", err)
		}
		found, err = fixture.appointments.GetByID(ctx, appointment.ID)
		if err != nil || found.PaymentStatus != models.AppointmentPaymentPaid || found.PaymentDueAt != nil {
			t.Fatalf("GetByID after verifying = %+v, %v; want paid with no deadline", found, err)
		}
	})

	t.Run("a rejected claim cancels the booking", func(t *testing.T) {
		appointment := pending(time.Hour)
		if err := fixture.appointments.CancelForRejectedPayment(ctx, appointment.ID, fixture.booker.ID); err != nil {
			t.Fatalf("CancelForRejectedPayment returned error: %v", err)
		}
		if err := fixture.appointments.CancelForRejectedPayment(ctx, appointment.ID, fixture.booker.ID); !errors.Is(err, apperrors.ErrAppointmentNotModifiable) {
			t.Fatalf("CancelForRejectedPayment twice error = %v, want %v", err, apperrors.ErrAppointmentNotModifiable)
		}
		found, err := fixture.appointments.GetByID(ctx, appointment.ID)
		if err != nil || found.Status != models.AppointmentStatusCancelled || found.CancelReason != models.CancelReasonPaymentRejected || found.PaymentStatus != models.AppointmentPaymentUnpaid || found.PaymentDueAt != nil {
			t.Fatalf("GetByID = %+v, %v; want cancelled for payment_rejected and unpaid", found, err)
		}
	})

	t.Run("a claim unchecked at the start expires as unverified", func(t *testing.T) {
		appointment := pending(2 * time.Hour)
		afterStart := appointment.StartsAt.Add(time.Minute)
		due, err := fixture.appointments.ListUnpaidDue(ctx, afterStart, 500)
		if err != nil || !slices.ContainsFunc(due, func(item *models.AppointmentDetails) bool { return item.ID == appointment.ID }) {
			t.Fatalf("ListUnpaidDue = %d rows, %v; want the pending booking", len(due), err)
		}
		if err := fixture.appointments.CancelUnpaid(ctx, appointment.ID, afterStart); err != nil {
			t.Fatalf("CancelUnpaid returned error: %v", err)
		}
		found, err := fixture.appointments.GetByID(ctx, appointment.ID)
		if err != nil || found.Status != models.AppointmentStatusCancelled || found.CancelReason != models.CancelReasonPaymentUnverified {
			t.Fatalf("GetByID = %+v, %v; want cancelled for payment_unverified", found, err)
		}
	})

	t.Run("a booking due at the clinic has no deadline", func(t *testing.T) {
		appointment := newUnpaidAppointment(t, fixture, startsAt.Add(3*time.Hour), time.Now().Add(10*time.Minute))
		if err := fixture.appointments.SetPaymentDue(ctx, appointment.ID, models.AppointmentPaymentUnpaid, models.AppointmentPaymentDueAtClinic, nil); err != nil {
			t.Fatalf("SetPaymentDue to due_at_clinic returned error: %v", err)
		}
		due, err := fixture.appointments.ListUnpaidDue(ctx, appointment.StartsAt.Add(time.Hour), 500)
		if err != nil || slices.ContainsFunc(due, func(item *models.AppointmentDetails) bool { return item.ID == appointment.ID }) {
			t.Fatalf("ListUnpaidDue = %d rows, %v; want the clinic booking left alone", len(due), err)
		}
	})
}

func TestDoctorRepositoryUpdateUPI(t *testing.T) {
	db := openTestDB(t)
	fixture := newAppointmentFixture(t, db)
	ctx := context.Background()
	doctors, err := repositories.NewDoctorRepository(db)
	if err != nil {
		t.Fatalf("NewDoctorRepository returned error: %v", err)
	}

	qrKey := "doctors/upi-qr-test"
	if err := doctors.UpdateUPI(ctx, fixture.doctor.ID, "asha@okbank", &qrKey); err != nil {
		t.Fatalf("UpdateUPI returned error: %v", err)
	}
	found, err := doctors.GetByID(ctx, fixture.doctor.ID)
	if err != nil || found.UPIID != "asha@okbank" || found.UPIQRStorageKey == nil || *found.UPIQRStorageKey != qrKey {
		t.Fatalf("GetByID = %+v, %v; want the upi id and qr", found, err)
	}

	if err := doctors.UpdateUPI(ctx, fixture.doctor.ID, "", nil); err != nil {
		t.Fatalf("UpdateUPI clearing returned error: %v", err)
	}
	found, err = doctors.GetByID(ctx, fixture.doctor.ID)
	if err != nil || found.UPIID != "" || found.UPIQRStorageKey != nil {
		t.Fatalf("GetByID after clearing = %+v, %v; want no upi", found, err)
	}
	if err := doctors.UpdateUPI(ctx, fixture.doctor.ID+1_000_000, "asha@okbank", nil); !errors.Is(err, apperrors.ErrDoctorNotFound) {
		t.Fatalf("UpdateUPI on a missing doctor error = %v, want %v", err, apperrors.ErrDoctorNotFound)
	}
}

func TestAppointmentRepositoryRemindsClinicBookings(t *testing.T) {
	db := openTestDB(t)
	fixture := newAppointmentFixture(t, db)
	ctx := context.Background()
	startsAt := time.Now().Add(180 * time.Hour).Truncate(time.Hour)

	atClinic := newUnpaidAppointment(t, fixture, startsAt, time.Now().Add(10*time.Minute))
	if err := fixture.appointments.SetPaymentDue(ctx, atClinic.ID, models.AppointmentPaymentUnpaid, models.AppointmentPaymentDueAtClinic, nil); err != nil {
		t.Fatalf("SetPaymentDue to due_at_clinic returned error: %v", err)
	}
	claimed := newUnpaidAppointment(t, fixture, startsAt.Add(time.Hour), time.Now().Add(10*time.Minute))
	claimedDue := claimed.StartsAt
	if err := fixture.appointments.SetPaymentDue(ctx, claimed.ID, models.AppointmentPaymentUnpaid, models.AppointmentPaymentPendingVerification, &claimedDue); err != nil {
		t.Fatalf("SetPaymentDue to pending_verification returned error: %v", err)
	}

	appointments, err := fixture.appointments.ListDueForReminder(ctx, models.ReminderWindowTwoHours, startsAt, startsAt.Add(24*time.Hour), 100)
	if err != nil {
		t.Fatalf("ListDueForReminder returned error: %v", err)
	}
	ids := []uint64{}
	for _, appointment := range appointments {
		if appointment.DoctorID == fixture.doctor.ID {
			ids = append(ids, appointment.ID)
		}
	}
	if len(ids) != 1 || ids[0] != atClinic.ID {
		t.Fatalf("ListDueForReminder = %v, want only the booking due at the clinic %d", ids, atClinic.ID)
	}
}
