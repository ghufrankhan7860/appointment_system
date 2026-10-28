package repositories_test

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/repositories"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestAppointmentRepositoryDoctorPatients(t *testing.T) {
	db := openTestDB(t)
	fixture := newAppointmentFixture(t, db)
	ctx := context.Background()
	patients, err := repositories.NewPatientRepository(db)
	if err != nil {
		t.Fatalf("NewPatientRepository returned error: %v", err)
	}

	stamp := time.Now().UnixNano()
	newPatient := func(name string) *models.Patient {
		created, err := patients.Create(ctx, &models.Patient{Name: name, Gender: models.GenderMale, BloodType: models.BloodTypeAPlus, DateOfBirth: time.Date(1990, 5, 5, 0, 0, 0, 0, time.UTC)})
		if err != nil {
			t.Fatalf("Create patient returned error: %v", err)
		}
		t.Cleanup(func() {
			db.Exec("DELETE FROM appointments WHERE patient_id = ?", created.ID)
			db.Exec("DELETE FROM patients WHERE id = ?", created.ID)
		})
		return created
	}
	second := newPatient(fmt.Sprintf("Zeta Second %d", stamp))
	proposalOnly := newPatient(fmt.Sprintf("Proposal Person %d", stamp))

	now := time.Now().UTC().Truncate(time.Minute)
	book := func(patient *models.Patient, startsAt time.Time, status models.AppointmentStatus, payment models.AppointmentPaymentStatus) {
		t.Helper()
		if _, err := fixture.appointments.Create(ctx, &models.Appointment{
			PatientID:       patient.ID,
			DoctorID:        fixture.doctor.ID,
			BookedByUserID:  fixture.booker.ID,
			StartsAt:        startsAt,
			EndsAt:          startsAt.Add(30 * time.Minute),
			Mode:            models.ConsultationModeInPerson,
			Status:          status,
			ConsultationFee: 50000,
			SlotDurationMin: 30,
			PaymentStatus:   payment,
		}); err != nil {
			t.Fatalf("Create appointment returned error: %v", err)
		}
	}
	book(fixture.patient, now.Add(-48*time.Hour), models.AppointmentStatusCompleted, models.AppointmentPaymentPaid)
	book(fixture.patient, now.Add(72*time.Hour), models.AppointmentStatusScheduled, models.AppointmentPaymentPaid)
	book(second, now.Add(-24*time.Hour), models.AppointmentStatusNoShow, models.AppointmentPaymentPaid)
	book(second, now.Add(24*time.Hour), models.AppointmentStatusScheduled, models.AppointmentPaymentUnpaid)
	book(second, now.Add(26*time.Hour), models.AppointmentStatusCancelled, models.AppointmentPaymentRefunded)
	book(proposalOnly, now.Add(96*time.Hour), models.AppointmentStatusRescheduleProposed, models.AppointmentPaymentPaid)

	listed, err := fixture.appointments.ListPatientsByDoctor(ctx, fixture.doctor.ID, "", now, 20, 0)
	if err != nil {
		t.Fatalf("ListPatientsByDoctor returned error: %v", err)
	}
	if len(listed) != 2 || listed[0].PatientID != fixture.patient.ID || listed[1].PatientID != second.ID {
		t.Fatalf("ListPatientsByDoctor = %+v, want the patient with a next visit first, then the no-show patient", listed)
	}
	first := listed[0]
	if first.Visits != 2 || first.LastAppointmentAt == nil || !first.LastAppointmentAt.Equal(now.Add(-48*time.Hour)) ||
		first.NextAppointmentAt == nil || !first.NextAppointmentAt.Equal(now.Add(72*time.Hour)) || first.Gender != models.GenderFemale {
		t.Fatalf("first patient = %+v, want 2 visits with last and next set", first)
	}
	if listed[1].Visits != 1 || listed[1].NextAppointmentAt != nil {
		t.Fatalf("second patient = %+v, want 1 visit and no next visit (the unpaid hold does not count)", listed[1])
	}

	searched, err := fixture.appointments.ListPatientsByDoctor(ctx, fixture.doctor.ID, "zeta", now, 20, 0)
	if err != nil || len(searched) != 1 || searched[0].PatientID != second.ID {
		t.Fatalf("ListPatientsByDoctor with q = %+v, %v; want only the Zeta patient", searched, err)
	}
	paged, err := fixture.appointments.ListPatientsByDoctor(ctx, fixture.doctor.ID, "", now, 1, 1)
	if err != nil || len(paged) != 1 || paged[0].PatientID != second.ID {
		t.Fatalf("ListPatientsByDoctor page 2 = %+v, %v; want the second patient", paged, err)
	}

	for _, check := range []struct {
		patientID uint64
		want      bool
	}{{fixture.patient.ID, true}, {second.ID, true}, {proposalOnly.ID, false}} {
		counted, err := fixture.appointments.HasCountedBooking(ctx, fixture.doctor.ID, check.patientID)
		if err != nil || counted != check.want {
			t.Fatalf("HasCountedBooking(%d) = %v, %v; want %v", check.patientID, counted, err, check.want)
		}
	}
}

func TestMedicalRecordRepositoryDoctorView(t *testing.T) {
	db := openTestDB(t)
	fixture := newAppointmentFixture(t, db)
	ctx := context.Background()
	records, err := repositories.NewMedicalRecordRepository(db)
	if err != nil {
		t.Fatalf("NewMedicalRecordRepository returned error: %v", err)
	}

	open := func(startsAt time.Time) *models.MedicalRecord {
		appointment, err := fixture.book(startsAt, 30)
		if err != nil {
			t.Fatalf("Create appointment returned error: %v", err)
		}
		created, err := records.Create(ctx, &models.MedicalRecord{AppointmentID: appointment.ID, PatientID: fixture.patient.ID, DoctorID: fixture.doctor.ID, CreatedByUserID: fixture.doctor.UserID})
		if err != nil {
			t.Fatalf("Create record returned error: %v", err)
		}
		return created
	}
	finalized := open(time.Now().Add(190 * time.Hour).Truncate(time.Hour))
	if err := records.Finalize(ctx, finalized.ID); err != nil {
		t.Fatalf("Finalize returned error: %v", err)
	}
	draft := open(time.Now().Add(192 * time.Hour).Truncate(time.Hour))

	own, err := records.ListForDoctorView(ctx, fixture.patient.ID, fixture.doctor.ID, 20, 0)
	if err != nil || len(own) != 2 {
		t.Fatalf("ListForDoctorView for the record's doctor = %d records, %v; want the finalized one and the own draft", len(own), err)
	}
	other, err := records.ListForDoctorView(ctx, fixture.patient.ID, fixture.doctor.ID+1_000_000, 20, 0)
	if err != nil || len(other) != 1 || other[0].ID != finalized.ID {
		t.Fatalf("ListForDoctorView for another doctor = %+v, %v; want only the finalized record, not draft %d", other, err, draft.ID)
	}
}

func TestAppointmentRepositoryCancelsProposal(t *testing.T) {
	db := openTestDB(t)
	fixture := newAppointmentFixture(t, db)
	ctx := context.Background()
	startsAt := time.Now().Add(200 * time.Hour).Truncate(time.Hour)

	original, err := fixture.book(startsAt, 30)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	placeholder, err := fixture.book(startsAt.Add(time.Hour), 30)
	if err != nil {
		t.Fatalf("Create placeholder returned error: %v", err)
	}
	proposal, err := fixture.appointments.CreateProposal(ctx, &models.RescheduleProposal{
		AppointmentID:            original.ID,
		PlaceholderAppointmentID: placeholder.ID,
		ProposedByUserID:         fixture.doctor.UserID,
		Reason:                   models.ProposalReasonOther,
		Status:                   models.ProposalStatusPending,
		ExpiresAt:                time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateProposal returned error: %v", err)
	}

	if err := fixture.appointments.CloseProposal(ctx, proposal.ID, models.ProposalStatusCancelled, fixture.booker.ID); err != nil {
		t.Fatalf("CloseProposal as cancelled returned error: %v", err)
	}
	if _, err := fixture.appointments.GetPendingProposal(ctx, original.ID); !errors.Is(err, apperrors.ErrProposalNotFound) {
		t.Fatalf("GetPendingProposal after cancelling error = %v, want %v", err, apperrors.ErrProposalNotFound)
	}
}

func TestUserRepositoryFindsLoginByOneColumn(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	users, err := repositories.NewUserRepository(db)
	if err != nil {
		t.Fatalf("NewUserRepository returned error: %v", err)
	}
	created, err := users.Create(ctx, newTestUser(t, db))
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if found, err := users.FindByEmail(ctx, " "+strings.ToUpper(created.Email)+" "); err != nil || found.ID != created.ID {
		t.Fatalf("FindByEmail = %+v, %v; want user %d", found, err, created.ID)
	}
	if found, err := users.FindByMobile(ctx, created.MobileNo); err != nil || found.ID != created.ID {
		t.Fatalf("FindByMobile = %+v, %v; want user %d", found, err, created.ID)
	}
	if _, err := users.FindByEmail(ctx, created.MobileNo); !errors.Is(err, apperrors.ErrInvalidCredentials) {
		t.Fatalf("FindByEmail with a mobile number error = %v, want %v", err, apperrors.ErrInvalidCredentials)
	}
	if _, err := users.FindByMobile(ctx, created.Email); !errors.Is(err, apperrors.ErrInvalidCredentials) {
		t.Fatalf("FindByMobile with an email error = %v, want %v", err, apperrors.ErrInvalidCredentials)
	}
}
