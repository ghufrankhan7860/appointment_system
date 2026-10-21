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

func scheduledAt(startsAt time.Time, status models.AppointmentStatus) *models.AppointmentDetails {
	return &models.AppointmentDetails{Appointment: models.Appointment{
		ID:              50,
		PatientID:       3,
		DoctorID:        5,
		BookedByUserID:  7,
		StartsAt:        startsAt,
		EndsAt:          startsAt.Add(30 * time.Minute),
		Mode:            models.ConsultationModeInPerson,
		Status:          status,
		ConsultationFee: 40000,
		SlotDurationMin: 30,
		PatientNotes:    "fever",
		PaymentStatus:   models.AppointmentPaymentPaid,
	}}
}

func TestAppointmentServiceCompleteAndNoShow(t *testing.T) {
	location := clinicLocation(t)
	nineAM := time.Date(2026, 10, 26, 9, 0, 0, 0, location)
	ownDoctor := clinicDoctor(models.VerificationStatusVerified)
	otherDoctor := clinicDoctor(models.VerificationStatusVerified)
	otherDoctor.ID = 6

	tests := []struct {
		name            string
		noShow          bool
		role            models.Role
		doctor          *models.Doctor
		status          models.AppointmentStatus
		now             time.Time
		updateStatusErr error
		wantStatus      models.AppointmentStatus
		wantEvent       string
		wantErr         error
	}{
		{name: "doctor completes after the start", role: models.RoleDoctor, doctor: ownDoctor, now: nineAM.Add(10 * time.Minute), wantStatus: models.AppointmentStatusCompleted, wantEvent: models.OutboxEventAppointmentCompleted},
		{name: "doctor marks a no-show after the start", noShow: true, role: models.RoleDoctor, doctor: ownDoctor, now: nineAM.Add(10 * time.Minute), wantStatus: models.AppointmentStatusNoShow, wantEvent: models.OutboxEventAppointmentNoShow},
		{name: "not started yet", role: models.RoleDoctor, doctor: ownDoctor, now: nineAM.Add(-time.Minute), wantErr: apperrors.ErrAppointmentNotModifiable},
		{name: "already cancelled", role: models.RoleDoctor, doctor: ownDoctor, status: models.AppointmentStatusCancelled, now: nineAM.Add(10 * time.Minute), wantErr: apperrors.ErrAppointmentNotModifiable},
		{name: "already completed cannot be a no-show", noShow: true, role: models.RoleDoctor, doctor: ownDoctor, status: models.AppointmentStatusCompleted, now: nineAM.Add(10 * time.Minute), wantErr: apperrors.ErrAppointmentNotModifiable},
		{name: "patient account", role: models.RolePatient, now: nineAM.Add(10 * time.Minute), wantErr: apperrors.ErrDoctorRoleRequired},
		{name: "doctor of another appointment", role: models.RoleDoctor, doctor: otherDoctor, now: nineAM.Add(10 * time.Minute), wantErr: apperrors.ErrAppointmentNotFound},
		{name: "changed meanwhile", role: models.RoleDoctor, doctor: ownDoctor, now: nineAM.Add(10 * time.Minute), updateStatusErr: apperrors.ErrAppointmentNotModifiable, wantErr: apperrors.ErrAppointmentNotModifiable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.status == "" {
				tt.status = models.AppointmentStatusScheduled
			}
			appointments := &fakeAppointmentRepo{byID: scheduledAt(nineAM, tt.status), updateStatusErr: tt.updateStatusErr}
			outbox := &fakeOutboxRepo{}
			userPatients := &fakeUserPatientRepo{activeErr: apperrors.ErrUserPatientNotFound}
			service := newTestAppointmentService(t, &fakeDoctorRepo{byUserID: tt.doctor}, appointments, outbox, userPatients, tt.now)

			closeAppointment := service.Complete
			if tt.noShow {
				closeAppointment = service.MarkNoShow
			}
			_, err := closeAppointment(context.Background(), 11, tt.role, 50)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("close error = %v, want %v", err, tt.wantErr)
				}
				if len(outbox.events) != 0 {
					t.Fatalf("close wrote %d outbox events after an error", len(outbox.events))
				}
				return
			}
			if err != nil {
				t.Fatalf("close returned error: %v", err)
			}
			if appointments.updateStatusTo != tt.wantStatus {
				t.Fatalf("close status = %q, want %q", appointments.updateStatusTo, tt.wantStatus)
			}
			change := appointments.statusChanges[0]
			if change.FromStatus != models.AppointmentStatusScheduled || change.ToStatus != tt.wantStatus || change.ChangedByUserID != 11 {
				t.Fatalf("close status change = %+v, want scheduled -> %s by 11", change, tt.wantStatus)
			}
			if len(outbox.events) != 1 || outbox.events[0].EventType != tt.wantEvent || outbox.events[0].Payload["status"] != tt.wantStatus {
				t.Fatalf("close outbox = %+v, want one %s event with the new status", outbox.events, tt.wantEvent)
			}
		})
	}
}

func TestAppointmentServiceReschedule(t *testing.T) {
	location := clinicLocation(t)
	nineAM := time.Date(2026, 10, 26, 9, 0, 0, 0, location)
	dayBefore := nineAM.Add(-21 * time.Hour)
	ownDoctor := clinicDoctor(models.VerificationStatusVerified)
	ownDoctor.ConsultationFee = 50000

	tests := []struct {
		name     string
		role     models.Role
		link     *models.UserPatient
		status   models.AppointmentStatus
		now      time.Time
		startsAt time.Time
		mode     models.ConsultationMode
		wantErr  error
	}{
		{name: "full access moves to a free slot", role: models.RolePatient, link: patientLink(models.AccessLevelFull)},
		{name: "appointments_only access moves to a free slot", role: models.RolePatient, link: patientLink(models.AccessLevelAppointmentsOnly)},
		{name: "read_only access cannot reschedule", role: models.RolePatient, link: patientLink(models.AccessLevelReadOnly), wantErr: apperrors.ErrRescheduleForbidden},
		{name: "doctor cannot use the patient reschedule", role: models.RoleDoctor, wantErr: apperrors.ErrRescheduleForbidden},
		{name: "user with no link", role: models.RolePatient, wantErr: apperrors.ErrAppointmentNotFound},
		{name: "inside the cancellation window", role: models.RolePatient, link: patientLink(models.AccessLevelFull), now: nineAM.Add(-time.Hour), wantErr: apperrors.ErrCancellationWindowPassed},
		{name: "already cancelled", role: models.RolePatient, link: patientLink(models.AccessLevelFull), status: models.AppointmentStatusCancelled, wantErr: apperrors.ErrAppointmentNotModifiable},
		{name: "time between slots", role: models.RolePatient, link: patientLink(models.AccessLevelFull), startsAt: nineAM.Add(70 * time.Minute), wantErr: apperrors.ErrSlotNotAvailable},
		{name: "its own current time is not free", role: models.RolePatient, link: patientLink(models.AccessLevelFull), startsAt: nineAM, wantErr: apperrors.ErrSlotNotAvailable},
		{name: "a mode the slot does not offer", role: models.RolePatient, link: patientLink(models.AccessLevelFull), mode: models.ConsultationModeVideo, wantErr: apperrors.ErrSlotNotAvailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.status == "" {
				tt.status = models.AppointmentStatusScheduled
			}
			if tt.now.IsZero() {
				tt.now = dayBefore
			}
			if tt.startsAt.IsZero() {
				tt.startsAt = nineAM.Add(time.Hour)
			}
			current := scheduledAt(nineAM, tt.status)
			appointments := &fakeAppointmentRepo{byID: current, booked: []*models.AppointmentDetails{current}}
			outbox := &fakeOutboxRepo{}
			userPatients := &fakeUserPatientRepo{activeResult: tt.link}
			if tt.link == nil {
				userPatients.activeErr = apperrors.ErrUserPatientNotFound
			}
			doctors := &fakeDoctorRepo{byID: ownDoctor, byUserID: ownDoctor}
			service := newTestAppointmentService(t, doctors, appointments, outbox, userPatients, tt.now)

			rescheduled, err := service.Reschedule(context.Background(), 7, tt.role, 50, tt.startsAt, tt.mode, "")
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Reschedule error = %v, want %v", err, tt.wantErr)
				}
				if appointments.cancelCalled || appointments.createInput != nil {
					t.Fatalf("Reschedule changed appointments after an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Reschedule returned error: %v", err)
			}
			if appointments.cancelReason != models.CancelReasonRescheduled {
				t.Fatalf("Reschedule cancel reason = %q, want rescheduled", appointments.cancelReason)
			}
			created := appointments.createInput
			if created.RescheduledFromID == nil || *created.RescheduledFromID != 50 || !created.StartsAt.Equal(tt.startsAt) {
				t.Fatalf("Reschedule created %+v, want the new time pointing back at 50", created)
			}
			if created.ConsultationFee != 40000 || created.Mode != models.ConsultationModeInPerson || created.PatientNotes != "fever" || created.BookedByUserID != 7 {
				t.Fatalf("Reschedule created fee %d, mode %q, notes %q, booked by %d; want the original fee, mode and notes booked by 7", created.ConsultationFee, created.Mode, created.PatientNotes, created.BookedByUserID)
			}
			if len(appointments.statusChanges) != 2 || appointments.statusChanges[0].ToStatus != models.AppointmentStatusCancelled || appointments.statusChanges[1].ToStatus != models.AppointmentStatusScheduled {
				t.Fatalf("Reschedule status changes = %d, want cancelled then scheduled", len(appointments.statusChanges))
			}
			if len(outbox.events) != 1 || outbox.events[0].EventType != models.OutboxEventAppointmentRescheduled || outbox.events[0].Payload["rescheduledFromId"] != uint64(50) {
				t.Fatalf("Reschedule outbox = %+v, want one rescheduled event from 50", outbox.events)
			}
			if rescheduled.RescheduledFromID == nil {
				t.Fatalf("Reschedule returned an appointment without rescheduledFromId")
			}
		})
	}
}

func TestAppointmentServiceIdempotency(t *testing.T) {
	location := clinicLocation(t)
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, location)
	nineAM := time.Date(2026, 10, 26, 9, 0, 0, 0, location)
	doctor := clinicDoctor(models.VerificationStatusVerified)
	request := func(key string, startsAt time.Time) *models.Appointment {
		return &models.Appointment{PatientID: 3, DoctorID: 5, StartsAt: startsAt, Mode: models.ConsultationModeInPerson, IdempotencyKey: key}
	}
	firstBooking := scheduledAt(nineAM, models.AppointmentStatusScheduled)
	firstBooking.IdempotencyKey = "key-1"

	t.Run("a new key is stored on the booking", func(t *testing.T) {
		appointments := &fakeAppointmentRepo{}
		outbox := &fakeOutboxRepo{}
		service := newTestAppointmentService(t, &fakeDoctorRepo{byID: doctor}, appointments, outbox, &fakeUserPatientRepo{activeResult: patientLink(models.AccessLevelFull)}, now)

		if _, err := service.Book(context.Background(), 7, request(" key-1 ", nineAM)); err != nil {
			t.Fatalf("Book returned error: %v", err)
		}
		if appointments.createInput.IdempotencyKey != "key-1" {
			t.Fatalf("Book key = %q, want key-1", appointments.createInput.IdempotencyKey)
		}
		if len(outbox.events) != 1 || outbox.events[0].EventType != models.OutboxEventAppointmentBooked {
			t.Fatalf("Book outbox = %+v, want one booked event", outbox.events)
		}
	})

	t.Run("a retry returns the first booking", func(t *testing.T) {
		appointments := &fakeAppointmentRepo{byKey: firstBooking, booked: []*models.AppointmentDetails{firstBooking}}
		service := newTestAppointmentService(t, &fakeDoctorRepo{byID: doctor}, appointments, &fakeOutboxRepo{}, &fakeUserPatientRepo{activeResult: patientLink(models.AccessLevelFull)}, now)

		booked, err := service.Book(context.Background(), 7, request("key-1", nineAM))
		if err != nil {
			t.Fatalf("Book returned error: %v", err)
		}
		if booked.ID != 50 || appointments.createInput != nil {
			t.Fatalf("Book returned %d and created %v, want the first booking and no new row", booked.ID, appointments.createInput)
		}
	})

	t.Run("the key on a different request", func(t *testing.T) {
		appointments := &fakeAppointmentRepo{byKey: firstBooking}
		service := newTestAppointmentService(t, &fakeDoctorRepo{byID: doctor}, appointments, &fakeOutboxRepo{}, &fakeUserPatientRepo{activeResult: patientLink(models.AccessLevelFull)}, now)

		if _, err := service.Book(context.Background(), 7, request("key-1", nineAM.Add(time.Hour))); !errors.Is(err, apperrors.ErrIdempotencyKeyReused) {
			t.Fatalf("Book error = %v, want %v", err, apperrors.ErrIdempotencyKeyReused)
		}
	})

	t.Run("two requests with one key at once", func(t *testing.T) {
		appointments := &fakeAppointmentRepo{createErr: apperrors.ErrIdempotencyKeyExists, clashWinner: firstBooking}
		service := newTestAppointmentService(t, &fakeDoctorRepo{byID: doctor}, appointments, &fakeOutboxRepo{}, &fakeUserPatientRepo{activeResult: patientLink(models.AccessLevelFull)}, now)

		booked, err := service.Book(context.Background(), 7, request("key-1", nineAM))
		if err != nil {
			t.Fatalf("Book returned error: %v", err)
		}
		if booked.ID != 50 {
			t.Fatalf("Book returned %d, want the winning booking 50", booked.ID)
		}
	})

	t.Run("a key that is too long", func(t *testing.T) {
		service := newTestAppointmentService(t, &fakeDoctorRepo{byID: doctor}, &fakeAppointmentRepo{}, &fakeOutboxRepo{}, &fakeUserPatientRepo{activeResult: patientLink(models.AccessLevelFull)}, now)

		if _, err := service.Book(context.Background(), 7, request(strings.Repeat("k", constant.MaxIdempotencyKeyLength+1), nineAM)); !errors.Is(err, apperrors.ErrInvalidAppointmentInput) {
			t.Fatalf("Book error = %v, want %v", err, apperrors.ErrInvalidAppointmentInput)
		}
	})

	t.Run("a reschedule retry after it went through", func(t *testing.T) {
		original := scheduledAt(nineAM, models.AppointmentStatusCancelled)
		moved := scheduledAt(nineAM.Add(time.Hour), models.AppointmentStatusScheduled)
		moved.ID = 51
		moved.IdempotencyKey = "key-2"
		movedFrom := uint64(50)
		moved.RescheduledFromID = &movedFrom
		appointments := &fakeAppointmentRepo{byID: original, byKey: moved}
		service := newTestAppointmentService(t, &fakeDoctorRepo{byID: doctor}, appointments, &fakeOutboxRepo{}, &fakeUserPatientRepo{activeResult: patientLink(models.AccessLevelFull)}, now)

		rescheduled, err := service.Reschedule(context.Background(), 7, models.RolePatient, 50, nineAM.Add(time.Hour), "", "key-2")
		if err != nil {
			t.Fatalf("Reschedule returned error: %v", err)
		}
		if rescheduled.ID != 51 || appointments.cancelCalled {
			t.Fatalf("Reschedule returned %d (cancelled again: %v), want the earlier result 51 and no change", rescheduled.ID, appointments.cancelCalled)
		}
	})
}

func TestAppointmentServiceCancelWritesOutboxEvent(t *testing.T) {
	location := clinicLocation(t)
	nineAM := time.Date(2026, 10, 26, 9, 0, 0, 0, location)
	appointments := &fakeAppointmentRepo{byID: scheduledAt(nineAM, models.AppointmentStatusScheduled)}
	outbox := &fakeOutboxRepo{}
	service := newTestAppointmentService(t, &fakeDoctorRepo{}, appointments, outbox, &fakeUserPatientRepo{activeResult: patientLink(models.AccessLevelFull)}, nineAM.Add(-24*time.Hour))

	if _, err := service.Cancel(context.Background(), 7, models.RolePatient, 50, ""); err != nil {
		t.Fatalf("Cancel returned error: %v", err)
	}
	if len(outbox.events) != 1 || outbox.events[0].EventType != models.OutboxEventAppointmentCancelled || outbox.events[0].Payload["status"] != models.AppointmentStatusCancelled {
		t.Fatalf("Cancel outbox = %+v, want one cancelled event", outbox.events)
	}
}
