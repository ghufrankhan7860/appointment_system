package repositories_test

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/repositories"
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
)

type appointmentFixture struct {
	appointments *repositories.AppointmentRepository
	booker       *models.User
	patient      *models.Patient
	doctor       *models.Doctor
}

func newAppointmentFixture(t *testing.T, db *gorm.DB) appointmentFixture {
	t.Helper()
	ctx := context.Background()

	users, err := repositories.NewUserRepository(db)
	if err != nil {
		t.Fatalf("NewUserRepository returned error: %v", err)
	}
	patients, err := repositories.NewPatientRepository(db)
	if err != nil {
		t.Fatalf("NewPatientRepository returned error: %v", err)
	}
	userPatients, err := repositories.NewUserPatientRepository(db)
	if err != nil {
		t.Fatalf("NewUserPatientRepository returned error: %v", err)
	}
	doctors, err := repositories.NewDoctorRepository(db)
	if err != nil {
		t.Fatalf("NewDoctorRepository returned error: %v", err)
	}
	appointments, err := repositories.NewAppointmentRepository(db)
	if err != nil {
		t.Fatalf("NewAppointmentRepository returned error: %v", err)
	}

	booker, err := users.Create(ctx, newTestUser(t, db))
	if err != nil {
		t.Fatalf("Create booker returned error: %v", err)
	}
	doctorUser, err := users.Create(ctx, newTestUser(t, db))
	if err != nil {
		t.Fatalf("Create doctor user returned error: %v", err)
	}
	patient, err := patients.Create(ctx, &models.Patient{
		Name:        "Appointment Patient",
		Gender:      models.GenderFemale,
		BloodType:   models.BloodTypeOPlus,
		DateOfBirth: time.Date(1960, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Create patient returned error: %v", err)
	}
	doctor, err := doctors.Create(ctx, &models.Doctor{
		UserID:              doctorUser.ID,
		FullName:            "Dr. Appointment",
		RegistrationNumber:  fmt.Sprintf("APPT-%d", time.Now().UnixNano()),
		RegistrationCouncil: "Test Council",
		Qualification:       "MBBS",
		SlotDurationMin:     30,
		VerificationStatus:  models.VerificationStatusVerified,
		IsAcceptingPatients: true,
	})
	if err != nil {
		t.Fatalf("Create doctor returned error: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM medical_record_access_log WHERE patient_id = ?", patient.ID)
		db.Exec("DELETE FROM medical_attachments WHERE patient_id = ?", patient.ID)
		db.Exec("DELETE FROM medical_records WHERE patient_id = ?", patient.ID)
		db.Exec("DELETE FROM appointment_reschedule_proposals WHERE appointment_id IN (SELECT id FROM appointments WHERE doctor_id = ?)", doctor.ID)
		db.Exec("DELETE FROM refunds WHERE payment_id IN (SELECT payments.id FROM payments JOIN appointments ON appointments.id = payments.appointment_id WHERE appointments.doctor_id = ?)", doctor.ID)
		db.Exec("DELETE FROM payment_status_history WHERE payment_id IN (SELECT payments.id FROM payments JOIN appointments ON appointments.id = payments.appointment_id WHERE appointments.doctor_id = ?)", doctor.ID)
		db.Exec("DELETE FROM payments WHERE appointment_id IN (SELECT id FROM appointments WHERE doctor_id = ?)", doctor.ID)
		db.Exec("DELETE FROM outbox WHERE aggregate_type = 'appointment' AND aggregate_id IN (SELECT id FROM appointments WHERE doctor_id = ?)", doctor.ID)
		db.Exec("DELETE FROM appointment_reminders WHERE appointment_id IN (SELECT id FROM appointments WHERE doctor_id = ?)", doctor.ID)
		db.Exec("DELETE FROM appointment_status_history WHERE appointment_id IN (SELECT id FROM appointments WHERE doctor_id = ?)", doctor.ID)
		db.Exec("DELETE FROM appointments WHERE doctor_id = ?", doctor.ID)
		db.Exec("DELETE FROM user_patient WHERE patient_id = ?", patient.ID)
		db.Exec("DELETE FROM patients WHERE id = ?", patient.ID)
		db.Exec("DELETE FROM doctors WHERE id = ?", doctor.ID)
	})

	if _, err := userPatients.Create(ctx, &models.UserPatient{
		UserID:              booker.ID,
		PatientID:           patient.ID,
		PatientRelationship: models.RelationshipChild,
		AccessLevel:         models.AccessLevelFull,
		Status:              models.UserPatientStatusActive,
	}); err != nil {
		t.Fatalf("Create user-patient returned error: %v", err)
	}

	return appointmentFixture{appointments: appointments, booker: booker, patient: patient, doctor: doctor}
}

func (fixture appointmentFixture) book(startsAt time.Time, minutes int) (*models.Appointment, error) {
	return fixture.appointments.Create(context.Background(), &models.Appointment{
		PatientID:       fixture.patient.ID,
		DoctorID:        fixture.doctor.ID,
		BookedByUserID:  fixture.booker.ID,
		StartsAt:        startsAt,
		EndsAt:          startsAt.Add(time.Duration(minutes) * time.Minute),
		Mode:            models.ConsultationModeInPerson,
		Status:          models.AppointmentStatusScheduled,
		ConsultationFee: 50000,
		SlotDurationMin: minutes,
		PaymentStatus:   models.AppointmentPaymentWaived,
	})
}

func TestAppointmentRepositoryRejectsOverlappingBookings(t *testing.T) {
	db := openTestDB(t)
	fixture := newAppointmentFixture(t, db)
	startsAt := time.Now().Add(48 * time.Hour).Truncate(time.Hour)

	first, err := fixture.book(startsAt, 30)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if _, err := fixture.book(startsAt.Add(15*time.Minute), 30); !errors.Is(err, apperrors.ErrSlotTaken) {
		t.Fatalf("Create overlapping error = %v, want %v", err, apperrors.ErrSlotTaken)
	}
	if _, err := fixture.book(startsAt.Add(30*time.Minute), 30); err != nil {
		t.Fatalf("Create back-to-back returned error: %v", err)
	}

	if err := fixture.appointments.Cancel(context.Background(), first.ID, fixture.booker.ID, models.CancelReasonPatientRequest); err != nil {
		t.Fatalf("Cancel returned error: %v", err)
	}
	if err := fixture.appointments.Cancel(context.Background(), first.ID, fixture.booker.ID, models.CancelReasonPatientRequest); !errors.Is(err, apperrors.ErrAppointmentNotModifiable) {
		t.Fatalf("Cancel twice error = %v, want %v", err, apperrors.ErrAppointmentNotModifiable)
	}
	if _, err := fixture.book(startsAt, 30); err != nil {
		t.Fatalf("Create after cancel returned error: %v", err)
	}
}

func TestAppointmentRepositoryConcurrentBookingsOfOneSlot(t *testing.T) {
	db := openTestDB(t)
	fixture := newAppointmentFixture(t, db)
	startsAt := time.Now().Add(72 * time.Hour).Truncate(time.Hour)

	const attempts = 10
	var wg sync.WaitGroup
	errs := make(chan error, attempts)
	for range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := fixture.book(startsAt, 30)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	booked, taken := 0, 0
	for err := range errs {
		switch {
		case err == nil:
			booked++
		case errors.Is(err, apperrors.ErrSlotTaken):
			taken++
		default:
			t.Fatalf("Create returned error: %v", err)
		}
	}
	if booked != 1 || taken != attempts-1 {
		t.Fatalf("concurrent Create booked %d and rejected %d, want 1 and %d", booked, taken, attempts-1)
	}
}

func TestAppointmentRepositoryReadsDetails(t *testing.T) {
	db := openTestDB(t)
	fixture := newAppointmentFixture(t, db)
	ctx := context.Background()
	startsAt := time.Now().Add(96 * time.Hour).Truncate(time.Hour)

	scheduled, err := fixture.book(startsAt, 30)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	cancelled, err := fixture.book(startsAt.Add(time.Hour), 30)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if err := fixture.appointments.Cancel(ctx, cancelled.ID, fixture.booker.ID, models.CancelReasonDoctorRequest); err != nil {
		t.Fatalf("Cancel returned error: %v", err)
	}

	details, err := fixture.appointments.GetByID(ctx, scheduled.ID)
	if err != nil {
		t.Fatalf("GetByID returned error: %v", err)
	}
	if details.PatientName != fixture.patient.Name || details.DoctorName != fixture.doctor.FullName || details.BookedByRelationship != models.RelationshipChild {
		t.Fatalf("GetByID details = %q, %q, %q; want the patient, doctor and child relationship", details.PatientName, details.DoctorName, details.BookedByRelationship)
	}
	if !details.StartsAt.Equal(startsAt) {
		t.Fatalf("GetByID starts at %v, want %v", details.StartsAt, startsAt)
	}

	byDoctor, err := fixture.appointments.ListByDoctorBetween(ctx, fixture.doctor.ID, startsAt, startsAt.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("ListByDoctorBetween returned error: %v", err)
	}
	if len(byDoctor) != 2 || byDoctor[1].Status != models.AppointmentStatusCancelled || byDoctor[1].CancelReason != models.CancelReasonDoctorRequest {
		t.Fatalf("ListByDoctorBetween = %d appointments, want the scheduled one then the cancelled one", len(byDoctor))
	}

	byUser, err := fixture.appointments.ListByUser(ctx, fixture.booker.ID, models.AppointmentFilter{Status: models.AppointmentStatusScheduled, Limit: 10})
	if err != nil {
		t.Fatalf("ListByUser returned error: %v", err)
	}
	if len(byUser) != 1 || byUser[0].ID != scheduled.ID {
		t.Fatalf("ListByUser = %d appointments, want only the scheduled one", len(byUser))
	}

	if _, err := fixture.appointments.GetByID(ctx, 0); !errors.Is(err, apperrors.ErrAppointmentNotFound) {
		t.Fatalf("GetByID error = %v, want %v", err, apperrors.ErrAppointmentNotFound)
	}
}

func TestAppointmentRepositoryIdempotencyRescheduleAndStatus(t *testing.T) {
	db := openTestDB(t)
	fixture := newAppointmentFixture(t, db)
	ctx := context.Background()
	startsAt := time.Now().Add(120 * time.Hour).Truncate(time.Hour)
	withKey := func(startsAt time.Time, key string, rescheduledFromID *uint64) (*models.Appointment, error) {
		return fixture.appointments.Create(ctx, &models.Appointment{
			PatientID:         fixture.patient.ID,
			DoctorID:          fixture.doctor.ID,
			BookedByUserID:    fixture.booker.ID,
			StartsAt:          startsAt,
			EndsAt:            startsAt.Add(30 * time.Minute),
			Mode:              models.ConsultationModeInPerson,
			Status:            models.AppointmentStatusScheduled,
			ConsultationFee:   50000,
			SlotDurationMin:   30,
			RescheduledFromID: rescheduledFromID,
			PaymentStatus:     models.AppointmentPaymentWaived,
			IdempotencyKey:    key,
		})
	}

	first, err := withKey(startsAt, "repo-key-1", nil)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, err := withKey(startsAt.Add(time.Hour), "repo-key-1", nil); !errors.Is(err, apperrors.ErrIdempotencyKeyExists) {
		t.Fatalf("Create with a used key error = %v, want %v", err, apperrors.ErrIdempotencyKeyExists)
	}
	if _, err := withKey(startsAt.Add(2*time.Hour), "", nil); err != nil {
		t.Fatalf("Create without a key returned error: %v", err)
	}
	if _, err := withKey(startsAt.Add(3*time.Hour), "", nil); err != nil {
		t.Fatalf("second Create without a key returned error: %v", err)
	}

	found, err := fixture.appointments.GetByIdempotencyKey(ctx, fixture.booker.ID, "repo-key-1")
	if err != nil || found.ID != first.ID || found.IdempotencyKey != "repo-key-1" {
		t.Fatalf("GetByIdempotencyKey = %v, %v; want appointment %d", found, err, first.ID)
	}
	if _, err := fixture.appointments.GetByIdempotencyKey(ctx, fixture.booker.ID, "repo-key-unused"); !errors.Is(err, apperrors.ErrAppointmentNotFound) {
		t.Fatalf("GetByIdempotencyKey error = %v, want %v", err, apperrors.ErrAppointmentNotFound)
	}

	if err := fixture.appointments.Cancel(ctx, first.ID, fixture.booker.ID, models.CancelReasonRescheduled); err != nil {
		t.Fatalf("Cancel returned error: %v", err)
	}
	moved, err := withKey(startsAt.Add(4*time.Hour), "repo-key-2", &first.ID)
	if err != nil {
		t.Fatalf("Create rescheduled returned error: %v", err)
	}
	movedDetails, err := fixture.appointments.GetByID(ctx, moved.ID)
	if err != nil || movedDetails.RescheduledFromID == nil || *movedDetails.RescheduledFromID != first.ID {
		t.Fatalf("GetByID rescheduled = %v, %v; want rescheduledFromId %d", movedDetails, err, first.ID)
	}

	if err := fixture.appointments.UpdateStatus(ctx, moved.ID, models.AppointmentStatusScheduled, models.AppointmentStatusCompleted); err != nil {
		t.Fatalf("UpdateStatus returned error: %v", err)
	}
	if err := fixture.appointments.UpdateStatus(ctx, moved.ID, models.AppointmentStatusScheduled, models.AppointmentStatusNoShow); !errors.Is(err, apperrors.ErrAppointmentNotModifiable) {
		t.Fatalf("UpdateStatus twice error = %v, want %v", err, apperrors.ErrAppointmentNotModifiable)
	}
}

func TestOutboxRepositoryCreate(t *testing.T) {
	db := openTestDB(t)
	outbox, err := repositories.NewOutboxRepository(db)
	if err != nil {
		t.Fatalf("NewOutboxRepository returned error: %v", err)
	}
	aggregateID := uint64(time.Now().UnixNano() % 1_000_000_000)
	t.Cleanup(func() {
		db.Exec("DELETE FROM outbox WHERE aggregate_type = ? AND aggregate_id = ?", "test", aggregateID)
	})

	if err := outbox.Create(context.Background(), &models.OutboxEvent{
		AggregateType: "test",
		AggregateID:   aggregateID,
		EventType:     models.OutboxEventAppointmentBooked,
		Payload:       map[string]any{"appointmentId": aggregateID, "status": models.AppointmentStatusScheduled},
	}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	var status string
	if err := db.Raw("SELECT payload->>'status' FROM outbox WHERE aggregate_type = ? AND aggregate_id = ? AND published_at IS NULL", "test", aggregateID).Scan(&status).Error; err != nil {
		t.Fatalf("read outbox row: %v", err)
	}
	if status != string(models.AppointmentStatusScheduled) {
		t.Fatalf("outbox payload status = %q, want scheduled", status)
	}
}
