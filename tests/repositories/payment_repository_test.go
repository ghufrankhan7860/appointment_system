package repositories_test

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/repositories"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func newUnpaidAppointment(t *testing.T, fixture appointmentFixture, startsAt time.Time, dueAt time.Time) *models.Appointment {
	t.Helper()

	appointment, err := fixture.appointments.Create(context.Background(), &models.Appointment{
		PatientID:       fixture.patient.ID,
		DoctorID:        fixture.doctor.ID,
		BookedByUserID:  fixture.booker.ID,
		StartsAt:        startsAt,
		EndsAt:          startsAt.Add(30 * time.Minute),
		Mode:            models.ConsultationModeInPerson,
		Status:          models.AppointmentStatusScheduled,
		ConsultationFee: 50000,
		SlotDurationMin: 30,
		PaymentStatus:   models.AppointmentPaymentUnpaid,
		PaymentDueAt:    &dueAt,
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	return appointment
}

func TestPaymentRepositoryLivePaymentsAndRefunds(t *testing.T) {
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

	appointment := newUnpaidAppointment(t, fixture, time.Now().Add(150*time.Hour).Truncate(time.Hour), time.Now().Add(10*time.Minute))
	open := func(order string) (*models.Payment, error) {
		return payments.Create(ctx, &models.Payment{
			AppointmentID:   appointment.ID,
			PayerUserID:     fixture.booker.ID,
			Amount:          50000,
			Currency:        "INR",
			Provider:        "none",
			ProviderOrderID: fmt.Sprintf("%s-%d", order, time.Now().UnixNano()),
			Status:          models.PaymentStatusCreated,
		})
	}

	first, err := open("order-a")
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, err := open("order-b"); !errors.Is(err, apperrors.ErrPaymentAlreadyExists) {
		t.Fatalf("second live Create error = %v, want %v", err, apperrors.ErrPaymentAlreadyExists)
	}
	if err := payments.CreateStatusChange(ctx, &models.PaymentStatusChange{PaymentID: first.ID, FromStatus: models.PaymentStatusCreated, ToStatus: models.PaymentStatusExpired}); err != nil {
		t.Fatalf("CreateStatusChange by the system returned error: %v", err)
	}

	if err := payments.UpdateStatus(ctx, first.ID, models.PaymentStatusCreated, models.PaymentStatusExpired, "", nil); err != nil {
		t.Fatalf("UpdateStatus returned error: %v", err)
	}
	if _, err := payments.GetActiveByAppointment(ctx, appointment.ID); !errors.Is(err, apperrors.ErrPaymentNotFound) {
		t.Fatalf("GetActiveByAppointment after expiry error = %v, want %v", err, apperrors.ErrPaymentNotFound)
	}
	second, err := open("order-c")
	if err != nil {
		t.Fatalf("Create after an expired payment returned error: %v", err)
	}

	capturedAt := time.Now()
	if err := payments.UpdateStatus(ctx, second.ID, models.PaymentStatusCreated, models.PaymentStatusCaptured, "pay-1", &capturedAt); err != nil {
		t.Fatalf("UpdateStatus to captured returned error: %v", err)
	}
	if err := payments.UpdateStatus(ctx, second.ID, models.PaymentStatusCreated, models.PaymentStatusCaptured, "pay-2", &capturedAt); !errors.Is(err, apperrors.ErrPaymentNotModifiable) {
		t.Fatalf("UpdateStatus twice error = %v, want %v", err, apperrors.ErrPaymentNotModifiable)
	}
	other := newUnpaidAppointment(t, fixture, time.Now().Add(152*time.Hour).Truncate(time.Hour), time.Now().Add(10*time.Minute))
	otherPayment, err := payments.Create(ctx, &models.Payment{AppointmentID: other.ID, PayerUserID: fixture.booker.ID, Amount: 50000, Currency: "INR", Provider: "none", ProviderOrderID: fmt.Sprintf("order-d-%d", time.Now().UnixNano()), Status: models.PaymentStatusCreated})
	if err != nil {
		t.Fatalf("Create for another appointment returned error: %v", err)
	}
	if err := payments.UpdateStatus(ctx, otherPayment.ID, models.PaymentStatusCreated, models.PaymentStatusCaptured, "pay-1", &capturedAt); !errors.Is(err, apperrors.ErrProviderPaymentUsed) {
		t.Fatalf("UpdateStatus with a used gateway payment id error = %v, want %v", err, apperrors.ErrProviderPaymentUsed)
	}

	latest, err := payments.GetLatestByAppointment(ctx, appointment.ID)
	if err != nil || latest.ID != second.ID || latest.ProviderPaymentID != "pay-1" || latest.CapturedAt == nil {
		t.Fatalf("GetLatestByAppointment = %+v, %v; want the captured payment with pay-1", latest, err)
	}

	refund, err := payments.CreateRefund(ctx, &models.Refund{PaymentID: second.ID, Amount: 50000, Reason: models.RefundReasonAdmin, Status: models.RefundStatusRequested})
	if err != nil {
		t.Fatalf("CreateRefund returned error: %v", err)
	}
	if _, err := payments.CreateRefund(ctx, &models.Refund{PaymentID: second.ID, Amount: 50000, Reason: models.RefundReasonAdmin, Status: models.RefundStatusRequested}); !errors.Is(err, apperrors.ErrRefundAlreadyExists) {
		t.Fatalf("second CreateRefund error = %v, want %v", err, apperrors.ErrRefundAlreadyExists)
	}
	if err := payments.FinishRefund(ctx, refund.ID, models.RefundStatusFailed, ""); err != nil {
		t.Fatalf("FinishRefund returned error: %v", err)
	}
	if _, err := payments.CreateRefund(ctx, &models.Refund{PaymentID: second.ID, Amount: 50000, Reason: models.RefundReasonAdmin, Status: models.RefundStatusRequested}); err != nil {
		t.Fatalf("CreateRefund after a failed one returned error: %v", err)
	}

	moved := newUnpaidAppointment(t, fixture, time.Now().Add(151*time.Hour).Truncate(time.Hour), time.Now().Add(10*time.Minute))
	if err := payments.MoveToAppointment(ctx, appointment.ID, moved.ID); err != nil {
		t.Fatalf("MoveToAppointment returned error: %v", err)
	}
	if latest, err := payments.GetLatestByAppointment(ctx, moved.ID); err != nil || latest.ID != second.ID {
		t.Fatalf("GetLatestByAppointment after the move = %+v, %v; want payment %d", latest, err, second.ID)
	}
}

func TestPaymentRepositoryProviderOrdersAndRefundQueue(t *testing.T) {
	db := openTestDB(t)
	fixture := newAppointmentFixture(t, db)
	ctx := context.Background()
	payments, err := repositories.NewPaymentRepository(db)
	if err != nil {
		t.Fatalf("NewPaymentRepository returned error: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM refunds WHERE payment_id IN (SELECT id FROM payments WHERE payer_user_id = ?)", fixture.booker.ID)
		db.Exec("DELETE FROM payments WHERE payer_user_id = ?", fixture.booker.ID)
	})

	appointment := newUnpaidAppointment(t, fixture, time.Now().Add(160*time.Hour).Truncate(time.Hour), time.Now().Add(10*time.Minute))
	orderID := fmt.Sprintf("sess-%d", time.Now().UnixNano())
	created, err := payments.Create(ctx, &models.Payment{AppointmentID: appointment.ID, PayerUserID: fixture.booker.ID, Amount: 50000, Currency: "INR", Provider: "youpi", ProviderOrderID: orderID, Status: models.PaymentStatusCreated})
	if err != nil {
		t.Fatalf("Create with the youpi provider returned error: %v", err)
	}

	found, err := payments.GetByProviderOrderID(ctx, "youpi", orderID)
	if err != nil || found.ID != created.ID {
		t.Fatalf("GetByProviderOrderID = %+v, %v; want payment %d", found, err, created.ID)
	}
	if _, err := payments.GetByProviderOrderID(ctx, "none", orderID); !errors.Is(err, apperrors.ErrPaymentNotFound) {
		t.Fatalf("GetByProviderOrderID for another provider error = %v, want %v", err, apperrors.ErrPaymentNotFound)
	}

	capturedAt := time.Now()
	if err := payments.UpdateStatus(ctx, created.ID, models.PaymentStatusCreated, models.PaymentStatusCaptured, fmt.Sprintf("utr-%d", time.Now().UnixNano()), &capturedAt); err != nil {
		t.Fatalf("UpdateStatus to captured returned error: %v", err)
	}
	refund, err := payments.CreateRefund(ctx, &models.Refund{PaymentID: created.ID, Amount: 50000, Reason: models.RefundReasonDoctorCancelled, Status: models.RefundStatusRequested})
	if err != nil {
		t.Fatalf("CreateRefund returned error: %v", err)
	}

	listsRefund := func(status models.RefundStatus) bool {
		listed, err := payments.ListRefunds(ctx, status, 200, 0)
		if err != nil {
			t.Fatalf("ListRefunds returned error: %v", err)
		}
		for _, candidate := range listed {
			if candidate.ID == refund.ID {
				return true
			}
		}
		return false
	}
	if !listsRefund(models.RefundStatusRequested) || listsRefund(models.RefundStatusSucceeded) {
		t.Fatalf("ListRefunds does not list refund %d as requested only", refund.ID)
	}

	if err := payments.FinishRefund(ctx, refund.ID, models.RefundStatusSucceeded, "412345678901"); err != nil {
		t.Fatalf("FinishRefund returned error: %v", err)
	}
	completed, err := payments.GetRefundByID(ctx, refund.ID)
	if err != nil || completed.Status != models.RefundStatusSucceeded || completed.ProviderRefundID != "412345678901" || completed.ProcessedAt == nil {
		t.Fatalf("GetRefundByID = %+v, %v; want a succeeded refund with its reference", completed, err)
	}
	if listsRefund(models.RefundStatusRequested) || !listsRefund(models.RefundStatusSucceeded) {
		t.Fatalf("ListRefunds does not list refund %d as succeeded only", refund.ID)
	}
	if _, err := payments.GetRefundByID(ctx, refund.ID+1_000_000); !errors.Is(err, apperrors.ErrRefundNotFound) {
		t.Fatalf("GetRefundByID for a missing refund error = %v, want %v", err, apperrors.ErrRefundNotFound)
	}
}

func TestAppointmentRepositoryPaidAndExpiredCannotBothWin(t *testing.T) {
	db := openTestDB(t)
	fixture := newAppointmentFixture(t, db)
	ctx := context.Background()
	now := time.Now()

	paidFirst := newUnpaidAppointment(t, fixture, now.Add(160*time.Hour).Truncate(time.Hour), now.Add(-time.Minute))
	expiredFirst := newUnpaidAppointment(t, fixture, now.Add(161*time.Hour).Truncate(time.Hour), now.Add(-time.Minute))
	stillHeld := newUnpaidAppointment(t, fixture, now.Add(162*time.Hour).Truncate(time.Hour), now.Add(10*time.Minute))

	due, err := fixture.appointments.ListUnpaidDue(ctx, now, 100)
	if err != nil {
		t.Fatalf("ListUnpaidDue returned error: %v", err)
	}
	dueIDs := map[uint64]bool{}
	for _, appointment := range due {
		dueIDs[appointment.ID] = true
	}
	if !dueIDs[paidFirst.ID] || !dueIDs[expiredFirst.ID] || dueIDs[stillHeld.ID] {
		t.Fatalf("ListUnpaidDue = %v, want the two past their hold and not the held one", dueIDs)
	}

	if err := fixture.appointments.MarkPaid(ctx, paidFirst.ID); err != nil {
		t.Fatalf("MarkPaid returned error: %v", err)
	}
	if err := fixture.appointments.CancelUnpaid(ctx, paidFirst.ID, now); !errors.Is(err, apperrors.ErrAppointmentNotModifiable) {
		t.Fatalf("CancelUnpaid after MarkPaid error = %v, want %v", err, apperrors.ErrAppointmentNotModifiable)
	}

	if err := fixture.appointments.CancelUnpaid(ctx, expiredFirst.ID, now); err != nil {
		t.Fatalf("CancelUnpaid returned error: %v", err)
	}
	if err := fixture.appointments.MarkPaid(ctx, expiredFirst.ID); !errors.Is(err, apperrors.ErrAppointmentNotModifiable) {
		t.Fatalf("MarkPaid after CancelUnpaid error = %v, want %v", err, apperrors.ErrAppointmentNotModifiable)
	}
	expired, err := fixture.appointments.GetByID(ctx, expiredFirst.ID)
	if err != nil || expired.Status != models.AppointmentStatusCancelled || expired.CancelReason != models.CancelReasonPaymentExpired || expired.CancelledByUserID != nil {
		t.Fatalf("expired appointment = %+v, %v; want cancelled for payment_expired by the system", expired, err)
	}

	if err := fixture.appointments.CancelUnpaid(ctx, stillHeld.ID, now); !errors.Is(err, apperrors.ErrAppointmentNotModifiable) {
		t.Fatalf("CancelUnpaid inside the hold error = %v, want %v", err, apperrors.ErrAppointmentNotModifiable)
	}
	if err := fixture.appointments.CreateStatusChange(ctx, &models.AppointmentStatusChange{AppointmentID: expiredFirst.ID, FromStatus: models.AppointmentStatusScheduled, ToStatus: models.AppointmentStatusCancelled}); err != nil {
		t.Fatalf("CreateStatusChange by the system returned error: %v", err)
	}
}
