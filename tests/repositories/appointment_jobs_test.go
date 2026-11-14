package repositories_test

import (
	"appointment_system/internal/apps/appointment/adapters/payment/none"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/repositories"
	"appointment_system/internal/apps/appointment/services"
	"context"
	"errors"
	"testing"
	"time"
)

func TestAppointmentRepositoryReminders(t *testing.T) {
	db := openTestDB(t)
	fixture := newAppointmentFixture(t, db)
	ctx := context.Background()
	startsAt := time.Now().Add(144 * time.Hour).Truncate(time.Hour)

	due, err := fixture.book(startsAt, 30)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	cancelled, err := fixture.book(startsAt.Add(time.Hour), 30)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if err := fixture.appointments.Cancel(ctx, cancelled.ID, fixture.booker.ID, models.CancelReasonPatientRequest); err != nil {
		t.Fatalf("Cancel returned error: %v", err)
	}

	// The window can hold other doctors' appointments left in the shared local
	// database, so only this fixture's appointments are looked at.
	listDue := func(window models.ReminderWindow) []uint64 {
		appointments, err := fixture.appointments.ListDueForReminder(ctx, window, startsAt, startsAt.Add(24*time.Hour), 100)
		if err != nil {
			t.Fatalf("ListDueForReminder returned error: %v", err)
		}
		ids := []uint64{}
		for _, appointment := range appointments {
			if appointment.DoctorID == fixture.doctor.ID {
				ids = append(ids, appointment.ID)
			}
		}
		return ids
	}

	if ids := listDue(models.ReminderWindowTwoHours); len(ids) != 1 || ids[0] != due.ID {
		t.Fatalf("ListDueForReminder = %v, want only the scheduled appointment %d", ids, due.ID)
	}

	created, err := fixture.appointments.CreateReminder(ctx, due.ID, models.ReminderWindowTwoHours)
	if err != nil || !created {
		t.Fatalf("CreateReminder = %v, %v; want a new reminder", created, err)
	}
	created, err = fixture.appointments.CreateReminder(ctx, due.ID, models.ReminderWindowTwoHours)
	if err != nil || created {
		t.Fatalf("CreateReminder again = %v, %v; want false without error", created, err)
	}

	if ids := listDue(models.ReminderWindowTwoHours); len(ids) != 0 {
		t.Fatalf("ListDueForReminder after the reminder = %v, want none", ids)
	}
	if ids := listDue(models.ReminderWindowDayBefore); len(ids) != 1 {
		t.Fatalf("ListDueForReminder for the other window = %v, want the appointment", ids)
	}
}

func TestOutboxRepositorySkipsRowsLockedByAnotherConsumer(t *testing.T) {
	db := openTestDB(t)
	outbox, err := repositories.NewOutboxRepository(db)
	if err != nil {
		t.Fatalf("NewOutboxRepository returned error: %v", err)
	}
	txManager, err := repositories.NewGORMTransactionManager(db)
	if err != nil {
		t.Fatalf("NewGORMTransactionManager returned error: %v", err)
	}
	ctx := context.Background()
	aggregateID := uint64(time.Now().UnixNano() % 1_000_000_000)
	t.Cleanup(func() {
		db.Exec("DELETE FROM outbox WHERE aggregate_type = ? AND aggregate_id = ?", "lock-test", aggregateID)
	})
	for range 2 {
		if err := outbox.Create(ctx, &models.OutboxEvent{AggregateType: "lock-test", AggregateID: aggregateID, EventType: "test", Payload: map[string]any{}}); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
	}

	locked := make(chan []uint64)
	release := make(chan struct{})
	done := make(chan error)
	go func() {
		done <- txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
			events, err := outbox.ListUnpublished(txCtx, constant.MaxListLimit)
			if err != nil {
				close(locked)
				return err
			}
			ids := []uint64{}
			for _, event := range events {
				ids = append(ids, event.ID)
			}
			locked <- ids
			<-release
			return nil
		})
	}()

	lockedIDs, ok := <-locked
	if !ok {
		t.Fatalf("first consumer failed: %v", <-done)
	}
	err = txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		events, err := outbox.ListUnpublished(txCtx, constant.MaxListLimit)
		if err != nil {
			return err
		}
		for _, event := range events {
			for _, id := range lockedIDs {
				if event.ID == id {
					t.Errorf("second consumer got event %d, which the first one holds", id)
				}
			}
		}
		return nil
	})
	close(release)
	if err != nil {
		t.Fatalf("second consumer returned error: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("first consumer returned error: %v", err)
	}

	if err := outbox.MarkPublished(ctx, lockedIDs); err != nil {
		t.Fatalf("MarkPublished returned error: %v", err)
	}
	var left int64
	db.Raw("SELECT COUNT(*) FROM outbox WHERE aggregate_type = ? AND aggregate_id = ? AND published_at IS NULL", "lock-test", aggregateID).Scan(&left)
	if left != 0 {
		t.Fatalf("unpublished test events after MarkPublished = %d, want 0", left)
	}
}

type failingCreateAppointmentRepository struct {
	*repositories.AppointmentRepository
}

func (repo failingCreateAppointmentRepository) Create(ctx context.Context, appointment *models.Appointment) (*models.Appointment, error) {
	return nil, errors.New("insert failed")
}

func TestAppointmentServiceRescheduleRollsBackWhenTheNewBookingFails(t *testing.T) {
	db := openTestDB(t)
	fixture := newAppointmentFixture(t, db)
	ctx := context.Background()

	location, err := time.LoadLocation(constant.DefaultTimezone)
	if err != nil {
		t.Fatalf("LoadLocation returned error: %v", err)
	}
	day := time.Now().In(location).AddDate(0, 0, 3)
	nineAM := time.Date(day.Year(), day.Month(), day.Day(), 9, 0, 0, 0, location)

	availabilityRepository, err := repositories.NewAvailabilityRepository(db)
	if err != nil {
		t.Fatalf("NewAvailabilityRepository returned error: %v", err)
	}
	if _, err := availabilityRepository.ReplaceRules(ctx, fixture.doctor.ID, []models.AvailabilityRule{
		{DayOfWeek: day.Weekday(), StartMinute: 9 * 60, EndMinute: 11 * 60, Mode: models.ConsultationModeInPerson},
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
	txManager, err := repositories.NewGORMTransactionManager(db)
	if err != nil {
		t.Fatalf("NewGORMTransactionManager returned error: %v", err)
	}
	availability, err := services.NewAvailabilityService(doctors, availabilityRepository, fixture.appointments, txManager, location, time.Now, discardLogger)
	if err != nil {
		t.Fatalf("NewAvailabilityService returned error: %v", err)
	}
	paymentRepository, err := repositories.NewPaymentRepository(db)
	if err != nil {
		t.Fatalf("NewPaymentRepository returned error: %v", err)
	}
	newService := func(appointments services.AppointmentRepository) *services.AppointmentService {
		payments, err := services.NewPaymentService(paymentRepository, appointments, outbox, userPatients, doctors, newTestClinicActor(t, db, doctors), none.New(), []models.PaymentMethod{models.PaymentMethodRazorpay}, newTestFileStorage(t), txManager, time.Now, discardLogger)
		if err != nil {
			t.Fatalf("NewPaymentService returned error: %v", err)
		}
		service, err := services.NewAppointmentService(appointments, outbox, userPatients, doctors, newTestClinicActor(t, db, doctors), availability, payments, newTestCallRoomRepository(t, db), newTestCallEventRepository(t, db), txManager, discardLogger)
		if err != nil {
			t.Fatalf("NewAppointmentService returned error: %v", err)
		}
		return service
	}

	booked, err := newService(fixture.appointments).Book(ctx, fixture.booker.ID, &models.Appointment{
		PatientID: fixture.patient.ID,
		DoctorID:  fixture.doctor.ID,
		StartsAt:  nineAM,
		Mode:      models.ConsultationModeInPerson,
	})
	if err != nil {
		t.Fatalf("Book returned error: %v", err)
	}

	failing := newService(failingCreateAppointmentRepository{fixture.appointments})
	if _, err := failing.Reschedule(ctx, fixture.booker.ID, models.RolePatient, booked.ID, nineAM.Add(time.Hour), "", ""); err == nil {
		t.Fatalf("Reschedule error = nil, want the insert failure")
	}

	original, err := fixture.appointments.GetByID(ctx, booked.ID)
	if err != nil {
		t.Fatalf("GetByID returned error: %v", err)
	}
	if original.Status != models.AppointmentStatusScheduled || original.CancelReason != "" {
		t.Fatalf("original after a failed reschedule = %s (%s), want still scheduled", original.Status, original.CancelReason)
	}
	var historyRows, eventRows int64
	db.Raw("SELECT COUNT(*) FROM appointment_status_history WHERE appointment_id = ?", booked.ID).Scan(&historyRows)
	db.Raw("SELECT COUNT(*) FROM outbox WHERE aggregate_type = 'appointment' AND event_type = ? AND (payload->>'rescheduledFromId')::bigint = ?", models.OutboxEventAppointmentRescheduled, booked.ID).Scan(&eventRows)
	if historyRows != 1 || eventRows != 0 {
		t.Fatalf("after a failed reschedule: %d history rows and %d rescheduled events, want 1 and 0", historyRows, eventRows)
	}
}
