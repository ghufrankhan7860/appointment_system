package repositories_test

import (
	"appointment_system/internal/apps/appointment/adapters/payment/none"
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/repositories"
	"appointment_system/internal/apps/appointment/services"
	"context"
	"errors"
	"testing"
	"time"
)

func TestAppointmentRepositoryProposals(t *testing.T) {
	db := openTestDB(t)
	fixture := newAppointmentFixture(t, db)
	ctx := context.Background()
	t.Cleanup(func() {
		db.Exec("DELETE FROM appointment_reschedule_proposals WHERE appointment_id IN (SELECT id FROM appointments WHERE doctor_id = ?)", fixture.doctor.ID)
	})
	startsAt := time.Now().Add(170 * time.Hour).Truncate(time.Hour)

	original, err := fixture.book(startsAt, 30)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	placeholder, err := fixture.appointments.Create(ctx, &models.Appointment{
		PatientID:         fixture.patient.ID,
		DoctorID:          fixture.doctor.ID,
		BookedByUserID:    fixture.booker.ID,
		StartsAt:          startsAt.Add(time.Hour),
		EndsAt:            startsAt.Add(90 * time.Minute),
		Mode:              models.ConsultationModeVideo,
		Status:            models.AppointmentStatusRescheduleProposed,
		ConsultationFee:   50000,
		SlotDurationMin:   30,
		RescheduledFromID: &original.ID,
		PaymentStatus:     models.AppointmentPaymentWaived,
	})
	if err != nil {
		t.Fatalf("Create placeholder returned error: %v", err)
	}
	if _, err := fixture.book(startsAt.Add(75*time.Minute), 30); !errors.Is(err, apperrors.ErrSlotTaken) {
		t.Fatalf("Create over a placeholder error = %v, want %v", err, apperrors.ErrSlotTaken)
	}

	expiresAt := time.Now().Add(-time.Minute)
	proposal := &models.RescheduleProposal{
		AppointmentID:            original.ID,
		PlaceholderAppointmentID: placeholder.ID,
		ProposedByUserID:         fixture.doctor.UserID,
		Reason:                   models.ProposalReasonClinicClosed,
		Status:                   models.ProposalStatusPending,
		ExpiresAt:                expiresAt,
	}
	created, err := fixture.appointments.CreateProposal(ctx, proposal)
	if err != nil {
		t.Fatalf("CreateProposal returned error: %v", err)
	}
	if _, err := fixture.appointments.CreateProposal(ctx, proposal); !errors.Is(err, apperrors.ErrAppointmentNotModifiable) {
		t.Fatalf("second pending CreateProposal error = %v, want %v", err, apperrors.ErrAppointmentNotModifiable)
	}

	pending, err := fixture.appointments.GetPendingProposal(ctx, original.ID)
	if err != nil || pending.ID != created.ID || !pending.NewStartsAt.Equal(startsAt.Add(time.Hour)) || pending.NewMode != models.ConsultationModeVideo {
		t.Fatalf("GetPendingProposal = %+v, %v; want the offered time and mode from the placeholder", pending, err)
	}
	expired, err := fixture.appointments.ListExpiredProposals(ctx, time.Now(), 100)
	if err != nil {
		t.Fatalf("ListExpiredProposals returned error: %v", err)
	}
	found := false
	for _, candidate := range expired {
		found = found || candidate.ID == created.ID
	}
	if !found {
		t.Fatalf("ListExpiredProposals did not return proposal %d", created.ID)
	}

	if err := fixture.appointments.CloseProposal(ctx, created.ID, models.ProposalStatusExpired, 0); err != nil {
		t.Fatalf("CloseProposal by the system returned error: %v", err)
	}
	if err := fixture.appointments.CloseProposal(ctx, created.ID, models.ProposalStatusAccepted, fixture.booker.ID); !errors.Is(err, apperrors.ErrAppointmentNotModifiable) {
		t.Fatalf("CloseProposal twice error = %v, want %v", err, apperrors.ErrAppointmentNotModifiable)
	}
	if _, err := fixture.appointments.GetPendingProposal(ctx, original.ID); !errors.Is(err, apperrors.ErrProposalNotFound) {
		t.Fatalf("GetPendingProposal after closing error = %v, want %v", err, apperrors.ErrProposalNotFound)
	}

	if err := fixture.appointments.Cancel(ctx, placeholder.ID, 0, models.CancelReasonProposalExpired); err != nil {
		t.Fatalf("Cancel of a placeholder by the system returned error: %v", err)
	}
	if _, err := fixture.book(startsAt.Add(75*time.Minute), 30); err != nil {
		t.Fatalf("Create after the placeholder was cancelled returned error: %v", err)
	}
}

func TestAppointmentServiceProposalFlowsOnPostgres(t *testing.T) {
	db := openTestDB(t)
	fixture := newAppointmentFixture(t, db)
	ctx := context.Background()
	t.Cleanup(func() {
		db.Exec("DELETE FROM appointment_reschedule_proposals WHERE appointment_id IN (SELECT id FROM appointments WHERE doctor_id = ?)", fixture.doctor.ID)
	})

	location, err := time.LoadLocation(constant.DefaultTimezone)
	if err != nil {
		t.Fatalf("LoadLocation returned error: %v", err)
	}
	day := time.Now().In(location).AddDate(0, 0, 4)
	at := func(hour int) time.Time {
		return time.Date(day.Year(), day.Month(), day.Day(), hour, 0, 0, 0, location)
	}

	availabilityRepository, err := repositories.NewAvailabilityRepository(db)
	if err != nil {
		t.Fatalf("NewAvailabilityRepository returned error: %v", err)
	}
	if _, err := availabilityRepository.ReplaceRules(ctx, fixture.doctor.ID, []models.AvailabilityRule{
		{DayOfWeek: day.Weekday(), StartMinute: 9 * 60, EndMinute: 13 * 60, Mode: models.ConsultationModeInPerson},
	}); err != nil {
		t.Fatalf("ReplaceRules returned error: %v", err)
	}
	doctors, err := repositories.NewDoctorRepository(db)
	if err != nil {
		t.Fatalf("NewDoctorRepository returned error: %v", err)
	}
	userPatients, err := repositories.NewUserPatientRepository(db)
	if err != nil {
		t.Fatalf("NewUserPatientRepository returned error: %v", err)
	}
	outbox, err := repositories.NewOutboxRepository(db)
	if err != nil {
		t.Fatalf("NewOutboxRepository returned error: %v", err)
	}
	paymentRepository, err := repositories.NewPaymentRepository(db)
	if err != nil {
		t.Fatalf("NewPaymentRepository returned error: %v", err)
	}
	txManager, err := repositories.NewGORMTransactionManager(db)
	if err != nil {
		t.Fatalf("NewGORMTransactionManager returned error: %v", err)
	}
	availability, err := services.NewAvailabilityService(doctors, availabilityRepository, fixture.appointments, txManager, location, time.Now)
	if err != nil {
		t.Fatalf("NewAvailabilityService returned error: %v", err)
	}
	payments, err := services.NewPaymentService(paymentRepository, fixture.appointments, outbox, userPatients, doctors, none.New(), txManager, time.Now)
	if err != nil {
		t.Fatalf("NewPaymentService returned error: %v", err)
	}
	service, err := services.NewAppointmentService(fixture.appointments, outbox, userPatients, doctors, availability, payments, txManager)
	if err != nil {
		t.Fatalf("NewAppointmentService returned error: %v", err)
	}

	book := func(hour int) *models.AppointmentDetails {
		t.Helper()
		booked, err := service.Book(ctx, fixture.booker.ID, &models.Appointment{PatientID: fixture.patient.ID, DoctorID: fixture.doctor.ID, StartsAt: at(hour), Mode: models.ConsultationModeInPerson})
		if err != nil {
			t.Fatalf("Book returned error: %v", err)
		}
		return booked
	}
	status := func(id uint64) models.AppointmentStatus {
		t.Helper()
		appointment, err := fixture.appointments.GetByID(ctx, id)
		if err != nil {
			t.Fatalf("GetByID returned error: %v", err)
		}
		return appointment.Status
	}

	accepted := book(9)
	if _, err := service.ProposeReschedule(ctx, fixture.doctor.UserID, models.RoleDoctor, accepted.ID, at(11), "", models.ProposalReasonEmergency, ""); err != nil {
		t.Fatalf("ProposeReschedule returned error: %v", err)
	}
	moved, err := service.AcceptReschedule(ctx, fixture.booker.ID, models.RolePatient, accepted.ID)
	if err != nil {
		t.Fatalf("AcceptReschedule returned error: %v", err)
	}
	if status(accepted.ID) != models.AppointmentStatusCancelled || moved.Status != models.AppointmentStatusScheduled || !moved.StartsAt.Equal(at(11)) {
		t.Fatalf("after accept: original %s, new %s at %v; want cancelled and scheduled at 11:00", status(accepted.ID), moved.Status, moved.StartsAt)
	}

	declined := book(10)
	proposal, err := service.ProposeReschedule(ctx, fixture.doctor.UserID, models.RoleDoctor, declined.ID, at(12), "", models.ProposalReasonPersonal, "")
	if err != nil {
		t.Fatalf("ProposeReschedule returned error: %v", err)
	}
	if _, err := service.DeclineReschedule(ctx, fixture.booker.ID, models.RolePatient, declined.ID); err != nil {
		t.Fatalf("DeclineReschedule returned error: %v", err)
	}
	if status(declined.ID) != models.AppointmentStatusScheduled || status(proposal.PlaceholderAppointmentID) != models.AppointmentStatusCancelled {
		t.Fatalf("after decline: original %s, placeholder %s; want scheduled and cancelled", status(declined.ID), status(proposal.PlaceholderAppointmentID))
	}

	slots, err := availability.ListSlots(ctx, fixture.doctor.ID, time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ListSlots returned error: %v", err)
	}
	for _, slot := range slots {
		if slot.StartsAt.Equal(at(12)) {
			return
		}
	}
	t.Fatalf("ListSlots after the decline does not offer 12:00 again")
}
