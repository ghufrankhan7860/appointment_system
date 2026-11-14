package services_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
)

func clinicLocation(t *testing.T) *time.Location {
	t.Helper()

	location, err := time.LoadLocation(constant.DefaultTimezone)
	if err != nil {
		t.Fatalf("LoadLocation returned error: %v", err)
	}

	return location
}

func newTestAppointmentService(
	t *testing.T,
	doctors *fakeDoctorRepo,
	appointments *fakeAppointmentRepo,
	outbox *fakeOutboxRepo,
	userPatients *fakeUserPatientRepo,
	now time.Time,
) *services.AppointmentService {
	t.Helper()

	appointmentService, _ := newTestAppointmentAndPaymentServices(t, doctors, appointments, outbox, userPatients, &fakePaymentRepo{}, &fakeGateway{}, now)
	return appointmentService
}

func newTestAppointmentAndPaymentServices(
	t *testing.T,
	doctors *fakeDoctorRepo,
	appointments *fakeAppointmentRepo,
	outbox *fakeOutboxRepo,
	userPatients *fakeUserPatientRepo,
	payments *fakePaymentRepo,
	gateway *fakeGateway,
	now time.Time,
) (*services.AppointmentService, *services.PaymentService) {
	t.Helper()

	return newTestServicesWithMethods(t, []models.PaymentMethod{models.PaymentMethodRazorpay}, &fakeFileStorage{}, doctors, appointments, outbox, userPatients, payments, gateway, now)
}

// newTestServicesWithMethods builds the services with PAYMENT_METHODS set to methods.
func newTestServicesWithMethods(
	t *testing.T,
	methods []models.PaymentMethod,
	storage *fakeFileStorage,
	doctors *fakeDoctorRepo,
	appointments *fakeAppointmentRepo,
	outbox *fakeOutboxRepo,
	userPatients *fakeUserPatientRepo,
	payments *fakePaymentRepo,
	gateway *fakeGateway,
	now time.Time,
) (*services.AppointmentService, *services.PaymentService) {
	t.Helper()

	return newTestServicesWithClinic(t, methods, storage, newTestClinicActor(doctors), doctors, appointments, outbox, userPatients, payments, gateway, now)
}

// newTestServicesWithClinic is newTestServicesWithMethods with the clinic desk
// resolver given, for desk and staff tests.
func newTestServicesWithClinic(
	t *testing.T,
	methods []models.PaymentMethod,
	storage *fakeFileStorage,
	clinic *services.ClinicActor,
	doctors *fakeDoctorRepo,
	appointments *fakeAppointmentRepo,
	outbox *fakeOutboxRepo,
	userPatients *fakeUserPatientRepo,
	payments *fakePaymentRepo,
	gateway *fakeGateway,
	now time.Time,
) (*services.AppointmentService, *services.PaymentService) {
	t.Helper()

	mondayMorning := &models.AvailabilityRule{DayOfWeek: time.Monday, StartMinute: 9 * 60, EndMinute: 11 * 60, Mode: models.ConsultationModeInPerson}
	availability, err := services.NewAvailabilityService(
		doctors,
		&fakeAvailabilityRepo{rules: []*models.AvailabilityRule{mondayMorning}},
		appointments,
		&fakeTxManager{},
		clinicLocation(t),
		func() time.Time { return now },
		discardLogger,
	)
	if err != nil {
		t.Fatalf("NewAvailabilityService returned error: %v", err)
	}

	paymentService, err := services.NewPaymentService(payments, appointments, outbox, userPatients, doctors, clinic, gateway, methods, storage, &fakeTxManager{}, func() time.Time { return now }, discardLogger)
	if err != nil {
		t.Fatalf("NewPaymentService returned error: %v", err)
	}
	service, err := services.NewAppointmentService(appointments, outbox, userPatients, doctors, clinic, availability, paymentService, &fakeCallRoomRepo{}, &fakeCallEventRepo{}, &fakeTxManager{}, discardLogger)
	if err != nil {
		t.Fatalf("NewAppointmentService returned error: %v", err)
	}

	return service, paymentService
}

func patientLink(accessLevel models.AccessLevel) *models.UserPatient {
	return &models.UserPatient{
		UserID:              7,
		PatientID:           3,
		PatientRelationship: models.RelationshipChild,
		AccessLevel:         accessLevel,
		Status:              models.UserPatientStatusActive,
	}
}

func TestNewAppointmentServiceValidation(t *testing.T) {
	availability, err := services.NewAvailabilityService(&fakeDoctorRepo{}, &fakeAvailabilityRepo{}, &fakeAppointmentRepo{}, &fakeTxManager{}, time.UTC, time.Now, discardLogger)
	if err != nil {
		t.Fatalf("NewAvailabilityService returned error: %v", err)
	}
	payments, err := services.NewPaymentService(&fakePaymentRepo{}, &fakeAppointmentRepo{}, &fakeOutboxRepo{}, &fakeUserPatientRepo{}, &fakeDoctorRepo{}, newTestClinicActor(&fakeDoctorRepo{}), &fakeGateway{}, []models.PaymentMethod{models.PaymentMethodRazorpay}, &fakeFileStorage{}, &fakeTxManager{}, time.Now, discardLogger)
	if err != nil {
		t.Fatalf("NewPaymentService returned error: %v", err)
	}

	tests := []struct {
		name         string
		appointments services.AppointmentRepository
		outbox       services.OutboxRepository
		userPatients services.UserPatientRepository
		doctors      services.DoctorRepository
		availability *services.AvailabilityService
		payments     *services.PaymentService
		callRooms    services.CallRoomRepository
		callEvents   services.CallEventRepository
		tx           services.TransactionManager
		wantErr      error
	}{
		{name: "nil appointment repository", outbox: &fakeOutboxRepo{}, userPatients: &fakeUserPatientRepo{}, doctors: &fakeDoctorRepo{}, availability: availability, payments: payments, callRooms: &fakeCallRoomRepo{}, callEvents: &fakeCallEventRepo{}, tx: &fakeTxManager{}, wantErr: apperrors.ErrNilAppointmentRepository},
		{name: "nil outbox repository", appointments: &fakeAppointmentRepo{}, userPatients: &fakeUserPatientRepo{}, doctors: &fakeDoctorRepo{}, availability: availability, payments: payments, callRooms: &fakeCallRoomRepo{}, callEvents: &fakeCallEventRepo{}, tx: &fakeTxManager{}, wantErr: apperrors.ErrNilOutboxRepository},
		{name: "nil user-patient repository", appointments: &fakeAppointmentRepo{}, outbox: &fakeOutboxRepo{}, doctors: &fakeDoctorRepo{}, availability: availability, payments: payments, callRooms: &fakeCallRoomRepo{}, callEvents: &fakeCallEventRepo{}, tx: &fakeTxManager{}, wantErr: apperrors.ErrNilUserPatientRepository},
		{name: "nil doctor repository", appointments: &fakeAppointmentRepo{}, outbox: &fakeOutboxRepo{}, userPatients: &fakeUserPatientRepo{}, availability: availability, payments: payments, callRooms: &fakeCallRoomRepo{}, callEvents: &fakeCallEventRepo{}, tx: &fakeTxManager{}, wantErr: apperrors.ErrNilDoctorRepository},
		{name: "nil availability service", appointments: &fakeAppointmentRepo{}, outbox: &fakeOutboxRepo{}, userPatients: &fakeUserPatientRepo{}, doctors: &fakeDoctorRepo{}, payments: payments, callRooms: &fakeCallRoomRepo{}, callEvents: &fakeCallEventRepo{}, tx: &fakeTxManager{}, wantErr: apperrors.ErrNilAvailabilityService},
		{name: "nil payment service", appointments: &fakeAppointmentRepo{}, outbox: &fakeOutboxRepo{}, userPatients: &fakeUserPatientRepo{}, doctors: &fakeDoctorRepo{}, availability: availability, callRooms: &fakeCallRoomRepo{}, callEvents: &fakeCallEventRepo{}, tx: &fakeTxManager{}, wantErr: apperrors.ErrNilPaymentService},
		{name: "nil transaction manager", appointments: &fakeAppointmentRepo{}, outbox: &fakeOutboxRepo{}, userPatients: &fakeUserPatientRepo{}, doctors: &fakeDoctorRepo{}, availability: availability, payments: payments, callRooms: &fakeCallRoomRepo{}, callEvents: &fakeCallEventRepo{}, wantErr: apperrors.ErrNilTransactionManager},
		{name: "nil call room repository", appointments: &fakeAppointmentRepo{}, outbox: &fakeOutboxRepo{}, userPatients: &fakeUserPatientRepo{}, doctors: &fakeDoctorRepo{}, availability: availability, payments: payments, callEvents: &fakeCallEventRepo{}, tx: &fakeTxManager{}, wantErr: apperrors.ErrNilCallRoomRepository},
		{name: "nil call event repository", appointments: &fakeAppointmentRepo{}, outbox: &fakeOutboxRepo{}, userPatients: &fakeUserPatientRepo{}, doctors: &fakeDoctorRepo{}, availability: availability, payments: payments, callRooms: &fakeCallRoomRepo{}, tx: &fakeTxManager{}, wantErr: apperrors.ErrNilCallEventRepository},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := services.NewAppointmentService(tt.appointments, tt.outbox, tt.userPatients, tt.doctors, newTestClinicActor(tt.doctors), tt.availability, tt.payments, tt.callRooms, tt.callEvents, tt.tx, discardLogger); !errors.Is(err, tt.wantErr) {
				t.Fatalf("NewAppointmentService error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestAppointmentServiceBook(t *testing.T) {
	location := clinicLocation(t)
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, location)
	nineAM := time.Date(2026, 10, 26, 9, 0, 0, 0, location)
	bookedNineAM := func(status models.AppointmentStatus) []*models.AppointmentDetails {
		return []*models.AppointmentDetails{{Appointment: models.Appointment{
			StartsAt: nineAM,
			EndsAt:   nineAM.Add(30 * time.Minute),
			Status:   status,
		}}}
	}

	tests := []struct {
		name         string
		link         *models.UserPatient
		doctorStatus models.VerificationStatus
		booked       []*models.AppointmentDetails
		createErr    error
		noDoctor     bool
		startsAt     time.Time
		mode         models.ConsultationMode
		wantErr      error
	}{
		{name: "full access books a free slot", link: patientLink(models.AccessLevelFull)},
		{name: "appointments_only access books a free slot", link: patientLink(models.AccessLevelAppointmentsOnly)},
		{name: "read_only access cannot book", link: patientLink(models.AccessLevelReadOnly), wantErr: apperrors.ErrPatientForbidden},
		{name: "no link to the patient", wantErr: apperrors.ErrPatientNotFound},
		{name: "time between slots", link: patientLink(models.AccessLevelFull), startsAt: nineAM.Add(10 * time.Minute), wantErr: apperrors.ErrSlotNotAvailable},
		{name: "mode the slot does not offer", link: patientLink(models.AccessLevelFull), mode: models.ConsultationModeVideo, wantErr: apperrors.ErrSlotNotAvailable},
		{name: "slot already booked", link: patientLink(models.AccessLevelFull), booked: bookedNineAM(models.AppointmentStatusScheduled), wantErr: apperrors.ErrSlotNotAvailable},
		{name: "cancelled booking frees the slot", link: patientLink(models.AccessLevelFull), booked: bookedNineAM(models.AppointmentStatusCancelled)},
		{name: "concurrent booking took the slot", link: patientLink(models.AccessLevelFull), createErr: apperrors.ErrSlotTaken, wantErr: apperrors.ErrSlotTaken},
		{name: "time in the past", link: patientLink(models.AccessLevelFull), startsAt: now.Add(-time.Hour), wantErr: apperrors.ErrInvalidAppointmentInput},
		{name: "time beyond the horizon", link: patientLink(models.AccessLevelFull), startsAt: nineAM.AddDate(0, 0, constant.SlotHorizonDays+7), wantErr: apperrors.ErrInvalidAppointmentInput},
		{name: "missing doctor", link: patientLink(models.AccessLevelFull), noDoctor: true, wantErr: apperrors.ErrInvalidAppointmentInput},
		{name: "doctor not verified", link: patientLink(models.AccessLevelFull), doctorStatus: models.VerificationStatusPending, wantErr: apperrors.ErrDoctorNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.doctorStatus == "" {
				tt.doctorStatus = models.VerificationStatusVerified
			}
			doctorID := uint64(5)
			if tt.noDoctor {
				doctorID = 0
			}
			if tt.startsAt.IsZero() {
				tt.startsAt = nineAM
			}
			if tt.mode == "" {
				tt.mode = models.ConsultationModeInPerson
			}

			doctor := clinicDoctor(tt.doctorStatus)
			doctor.ConsultationFee = 50000
			userPatients := &fakeUserPatientRepo{activeResult: tt.link}
			if tt.link == nil {
				userPatients.activeErr = apperrors.ErrUserPatientNotFound
			}
			appointments := &fakeAppointmentRepo{booked: tt.booked, createErr: tt.createErr}
			service := newTestAppointmentService(t, &fakeDoctorRepo{byID: doctor, byUserID: doctor}, appointments, &fakeOutboxRepo{}, userPatients, now)

			booked, err := service.Book(context.Background(), 7, &models.Appointment{
				PatientID:    3,
				DoctorID:     doctorID,
				StartsAt:     tt.startsAt,
				Mode:         tt.mode,
				PatientNotes: "  fever since Monday  ",
			})
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Book error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Book returned error: %v", err)
			}

			created := appointments.createInput
			if created.Status != models.AppointmentStatusScheduled || created.BookedByUserID != 7 {
				t.Fatalf("Book created status %q booked by %d, want scheduled by 7", created.Status, created.BookedByUserID)
			}
			if !created.EndsAt.Equal(nineAM.Add(30*time.Minute)) || created.SlotDurationMin != 30 || created.ConsultationFee != 50000 {
				t.Fatalf("Book created end %v, duration %d, fee %d; want the slot end and doctor snapshots", created.EndsAt, created.SlotDurationMin, created.ConsultationFee)
			}
			if created.PatientNotes != "fever since Monday" {
				t.Fatalf("Book notes = %q, want trimmed notes", created.PatientNotes)
			}
			if len(appointments.statusChanges) != 1 || appointments.statusChanges[0].ToStatus != models.AppointmentStatusScheduled {
				t.Fatalf("Book status changes = %v, want one change to scheduled", appointments.statusChanges)
			}
			if booked.ID != 50 {
				t.Fatalf("Book returned id %d, want 50", booked.ID)
			}
		})
	}
}

func TestAppointmentServiceCancel(t *testing.T) {
	location := clinicLocation(t)
	nineAM := time.Date(2026, 10, 26, 9, 0, 0, 0, location)
	dayBefore := nineAM.Add(-21 * time.Hour)
	insideWindow := nineAM.Add(-90 * time.Minute)
	ownDoctor := clinicDoctor(models.VerificationStatusVerified)
	otherDoctor := clinicDoctor(models.VerificationStatusVerified)
	otherDoctor.ID = 6

	tests := []struct {
		name       string
		role       models.Role
		link       *models.UserPatient
		doctor     *models.Doctor
		status     models.AppointmentStatus
		now        time.Time
		wantReason models.CancelReason
		wantErr    error
	}{
		{name: "patient side with full access", role: models.RolePatient, link: patientLink(models.AccessLevelFull), now: dayBefore, wantReason: models.CancelReasonPatientRequest},
		{name: "patient side with appointments_only access", role: models.RolePatient, link: patientLink(models.AccessLevelAppointmentsOnly), now: dayBefore, wantReason: models.CancelReasonPatientRequest},
		{name: "read_only access cannot cancel", role: models.RolePatient, link: patientLink(models.AccessLevelReadOnly), now: dayBefore, wantErr: apperrors.ErrPatientForbidden},
		{name: "patient side inside the cancellation window", role: models.RolePatient, link: patientLink(models.AccessLevelFull), now: insideWindow, wantErr: apperrors.ErrCancellationWindowPassed},
		{name: "doctor inside the cancellation window", role: models.RoleDoctor, doctor: ownDoctor, now: insideWindow, wantReason: models.CancelReasonDoctorRequest},
		{name: "doctor after the start", role: models.RoleDoctor, doctor: ownDoctor, now: nineAM.Add(5 * time.Minute), wantErr: apperrors.ErrAppointmentNotModifiable},
		{name: "already cancelled", role: models.RolePatient, link: patientLink(models.AccessLevelFull), status: models.AppointmentStatusCancelled, now: dayBefore, wantErr: apperrors.ErrAppointmentNotModifiable},
		{name: "user with no link", role: models.RolePatient, now: dayBefore, wantErr: apperrors.ErrAppointmentNotFound},
		{name: "doctor of another appointment", role: models.RoleDoctor, doctor: otherDoctor, now: dayBefore, wantErr: apperrors.ErrAppointmentNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.status == "" {
				tt.status = models.AppointmentStatusScheduled
			}
			userPatients := &fakeUserPatientRepo{activeResult: tt.link}
			if tt.link == nil {
				userPatients.activeErr = apperrors.ErrUserPatientNotFound
			}
			appointments := &fakeAppointmentRepo{byID: &models.AppointmentDetails{Appointment: models.Appointment{
				ID:        50,
				PatientID: 3,
				DoctorID:  5,
				StartsAt:  nineAM,
				EndsAt:    nineAM.Add(30 * time.Minute),
				Status:    tt.status,
			}}}
			service := newTestAppointmentService(t, &fakeDoctorRepo{byUserID: tt.doctor}, appointments, &fakeOutboxRepo{}, userPatients, tt.now)

			_, err := service.Cancel(context.Background(), 7, tt.role, 50, " running late ")
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Cancel error = %v, want %v", err, tt.wantErr)
				}
				if appointments.cancelCalled {
					t.Fatalf("Cancel changed the appointment after an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Cancel returned error: %v", err)
			}
			if appointments.cancelReason != tt.wantReason {
				t.Fatalf("Cancel reason = %q, want %q", appointments.cancelReason, tt.wantReason)
			}
			change := appointments.statusChanges[0]
			if change.FromStatus != models.AppointmentStatusScheduled || change.ToStatus != models.AppointmentStatusCancelled || change.Reason != "running late" {
				t.Fatalf("Cancel status change = %+v, want scheduled -> cancelled with the trimmed reason", change)
			}
		})
	}
}

func TestAppointmentServiceGetByIDHidesOtherAppointments(t *testing.T) {
	appointments := &fakeAppointmentRepo{byID: &models.AppointmentDetails{Appointment: models.Appointment{ID: 50, PatientID: 3, DoctorID: 5}}}

	readOnly := newTestAppointmentService(t, &fakeDoctorRepo{}, appointments, &fakeOutboxRepo{}, &fakeUserPatientRepo{activeResult: patientLink(models.AccessLevelReadOnly)}, time.Now())
	if _, err := readOnly.GetByID(context.Background(), 7, models.RolePatient, 50); err != nil {
		t.Fatalf("GetByID with read_only access returned error: %v", err)
	}

	stranger := newTestAppointmentService(t, &fakeDoctorRepo{}, appointments, &fakeOutboxRepo{}, &fakeUserPatientRepo{activeErr: apperrors.ErrUserPatientNotFound}, time.Now())
	if _, err := stranger.GetByID(context.Background(), 8, models.RolePatient, 50); !errors.Is(err, apperrors.ErrAppointmentNotFound) {
		t.Fatalf("GetByID error = %v, want %v", err, apperrors.ErrAppointmentNotFound)
	}
}

func TestAppointmentServiceList(t *testing.T) {
	from := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		filter    models.AppointmentFilter
		wantLimit int
		wantErr   error
	}{
		{name: "default page size", filter: models.AppointmentFilter{Status: models.AppointmentStatusScheduled}, wantLimit: constant.DefaultListLimit},
		{name: "page size capped", filter: models.AppointmentFilter{Limit: constant.MaxListLimit + 1}, wantLimit: constant.MaxListLimit},
		{name: "unknown status", filter: models.AppointmentFilter{Status: "booked"}, wantErr: apperrors.ErrInvalidAppointmentInput},
		{name: "to before from", filter: models.AppointmentFilter{From: from, To: from.Add(-time.Hour)}, wantErr: apperrors.ErrInvalidAppointmentInput},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appointments := &fakeAppointmentRepo{}
			service := newTestAppointmentService(t, &fakeDoctorRepo{}, appointments, &fakeOutboxRepo{}, &fakeUserPatientRepo{}, time.Now())

			_, err := service.List(context.Background(), 7, tt.filter)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("List error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("List returned error: %v", err)
			}
			if appointments.listFilter.Limit != tt.wantLimit {
				t.Fatalf("List limit = %d, want %d", appointments.listFilter.Limit, tt.wantLimit)
			}
		})
	}
}

func TestAppointmentServiceListForDoctor(t *testing.T) {
	location := clinicLocation(t)
	doctor := clinicDoctor(models.VerificationStatusVerified)
	appointments := &fakeAppointmentRepo{}
	service := newTestAppointmentService(t, &fakeDoctorRepo{byUserID: doctor}, appointments, &fakeOutboxRepo{}, &fakeUserPatientRepo{}, time.Now())

	if _, err := service.ListForDoctor(context.Background(), 7, models.RolePatient, time.Now()); !errors.Is(err, apperrors.ErrDoctorRoleRequired) {
		t.Fatalf("ListForDoctor error = %v, want %v", err, apperrors.ErrDoctorRoleRequired)
	}

	date := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	if _, err := service.ListForDoctor(context.Background(), 11, models.RoleDoctor, date); err != nil {
		t.Fatalf("ListForDoctor returned error: %v", err)
	}
	wantFrom := time.Date(2026, 10, 26, 0, 0, 0, 0, location)
	if !appointments.listFrom.Equal(wantFrom) || !appointments.listTo.Equal(wantFrom.AddDate(0, 0, 1)) {
		t.Fatalf("ListForDoctor range = %v to %v, want the clinic day from %v", appointments.listFrom, appointments.listTo, wantFrom)
	}
}

func TestAvailabilityServiceListSlotsSkipsBookedTimes(t *testing.T) {
	location := clinicLocation(t)
	nineAM := time.Date(2026, 10, 26, 9, 0, 0, 0, location)
	doctor := clinicDoctor(models.VerificationStatusVerified)
	appointments := &fakeAppointmentRepo{booked: []*models.AppointmentDetails{
		{Appointment: models.Appointment{StartsAt: nineAM, EndsAt: nineAM.Add(45 * time.Minute), Status: models.AppointmentStatusScheduled}},
		{Appointment: models.Appointment{StartsAt: nineAM.Add(time.Hour), EndsAt: nineAM.Add(90 * time.Minute), Status: models.AppointmentStatusCancelled}},
	}}
	availability, err := services.NewAvailabilityService(
		&fakeDoctorRepo{byID: doctor},
		&fakeAvailabilityRepo{rules: []*models.AvailabilityRule{{DayOfWeek: time.Monday, StartMinute: 9 * 60, EndMinute: 11 * 60, Mode: models.ConsultationModeInPerson}}},
		appointments,
		&fakeTxManager{},
		location,
		func() time.Time { return nineAM.Add(-24 * time.Hour) },
		discardLogger,
	)
	if err != nil {
		t.Fatalf("NewAvailabilityService returned error: %v", err)
	}

	slots, err := availability.ListSlots(context.Background(), 5, time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ListSlots returned error: %v", err)
	}

	got := []string{}
	for _, slot := range slots {
		got = append(got, slot.StartsAt.In(location).Format(constant.ClockLayout))
	}
	want := []string{"10:00", "10:30"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("ListSlots = %v, want %v (a 45 minute booking takes two 30 minute slots; a cancelled one takes none)", got, want)
	}
}
