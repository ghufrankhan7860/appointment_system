package repositories_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/repositories"
)

func TestStaffRepositoryLinksAndInvites(t *testing.T) {
	db := openTestDB(t)
	fixture := newAppointmentFixture(t, db)
	ctx := context.Background()
	users, err := repositories.NewUserRepository(db)
	if err != nil {
		t.Fatalf("NewUserRepository returned error: %v", err)
	}
	staffRepository, err := repositories.NewStaffRepository(db)
	if err != nil {
		t.Fatalf("NewStaffRepository returned error: %v", err)
	}
	invites, err := repositories.NewStaffInviteRepository(db)
	if err != nil {
		t.Fatalf("NewStaffInviteRepository returned error: %v", err)
	}

	staffUser := newTestUser(t, db)
	staffUser.Role = models.RoleStaff
	staffUser, err = users.Create(ctx, staffUser)
	if err != nil {
		t.Fatalf("Create staff user returned error: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM doctor_staff_invites WHERE doctor_id = ?", fixture.doctor.ID)
		db.Exec("DELETE FROM doctor_staff WHERE doctor_id = ?", fixture.doctor.ID)
	})

	link, err := staffRepository.Create(ctx, &models.DoctorStaff{DoctorID: fixture.doctor.ID, UserID: staffUser.ID})
	if err != nil || link.Status != models.StaffStatusActive {
		t.Fatalf("Create = %+v, %v; want an active staff link", link, err)
	}
	if _, err := staffRepository.Create(ctx, &models.DoctorStaff{DoctorID: fixture.doctor.ID, UserID: staffUser.ID}); !errors.Is(err, apperrors.ErrStaffAlreadyLinked) {
		t.Fatalf("second Create error = %v, want %v", err, apperrors.ErrStaffAlreadyLinked)
	}
	members, err := staffRepository.ListActiveByDoctor(ctx, fixture.doctor.ID)
	if err != nil || len(members) != 1 || members[0].UserID != staffUser.ID || members[0].Email != staffUser.Email {
		t.Fatalf("ListActiveByDoctor = %+v, %v; want the staff member with their email", members, err)
	}
	if err := staffRepository.Remove(ctx, fixture.doctor.ID, staffUser.ID, fixture.doctor.UserID, time.Now()); err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}
	if _, err := staffRepository.GetActiveByUser(ctx, staffUser.ID); !errors.Is(err, apperrors.ErrStaffNotFound) {
		t.Fatalf("GetActiveByUser after removal error = %v, want %v", err, apperrors.ErrStaffNotFound)
	}
	if err := staffRepository.Remove(ctx, fixture.doctor.ID, staffUser.ID, fixture.doctor.UserID, time.Now()); !errors.Is(err, apperrors.ErrStaffNotFound) {
		t.Fatalf("Remove twice error = %v, want %v", err, apperrors.ErrStaffNotFound)
	}
	if _, err := staffRepository.Create(ctx, &models.DoctorStaff{DoctorID: fixture.doctor.ID, UserID: staffUser.ID}); err != nil {
		t.Fatalf("Create after removal returned error: %v; a removed staff member can join again", err)
	}

	hash := fmt.Sprintf("%064d", time.Now().UnixNano())
	invite, err := invites.Create(ctx, &models.StaffInvite{DoctorID: fixture.doctor.ID, CreatedByUserID: fixture.doctor.UserID, CodeHash: hash, Status: models.StaffInviteStatusPending, ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatalf("Create invite returned error: %v", err)
	}
	if _, err := invites.Create(ctx, &models.StaffInvite{DoctorID: fixture.doctor.ID, CreatedByUserID: fixture.doctor.UserID, CodeHash: hash, Status: models.StaffInviteStatusPending, ExpiresAt: time.Now().Add(time.Hour)}); !errors.Is(err, apperrors.ErrStaffInviteCodeExists) {
		t.Fatalf("Create with the same code error = %v, want %v", err, apperrors.ErrStaffInviteCodeExists)
	}
	if count, err := invites.CountPendingByDoctor(ctx, fixture.doctor.ID, time.Now()); err != nil || count != 1 {
		t.Fatalf("CountPendingByDoctor = %d, %v; want 1", count, err)
	}
	found, err := invites.GetByCodeHashForUpdate(ctx, hash)
	if err != nil || found.ID != invite.ID {
		t.Fatalf("GetByCodeHashForUpdate = %+v, %v; want invite %d", found, err, invite.ID)
	}
	if err := invites.MarkRedeemed(ctx, invite.ID, staffUser.ID, time.Now()); err != nil {
		t.Fatalf("MarkRedeemed returned error: %v", err)
	}
	if err := invites.Revoke(ctx, fixture.doctor.ID, invite.ID); !errors.Is(err, apperrors.ErrStaffInviteNotFound) {
		t.Fatalf("Revoke a redeemed invite error = %v, want %v", err, apperrors.ErrStaffInviteNotFound)
	}
}

func TestPatientPassAndClinicPatientRepositories(t *testing.T) {
	db := openTestDB(t)
	fixture := newAppointmentFixture(t, db)
	ctx := context.Background()
	passes, err := repositories.NewPatientPassRepository(db)
	if err != nil {
		t.Fatalf("NewPatientPassRepository returned error: %v", err)
	}
	clinicPatients, err := repositories.NewClinicPatientRepository(db)
	if err != nil {
		t.Fatalf("NewClinicPatientRepository returned error: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM patient_passes WHERE patient_id = ?", fixture.patient.ID)
		db.Exec("DELETE FROM clinic_patients WHERE patient_id = ?", fixture.patient.ID)
	})

	stamp := time.Now().UnixNano()
	first := fmt.Sprintf("%064d", stamp)
	second := fmt.Sprintf("%064d", stamp+1)
	if _, err := passes.Replace(ctx, &models.PatientPass{PatientID: fixture.patient.ID, CreatedByUserID: fixture.booker.ID, TokenHash: "t" + first[1:], CodeHash: "c" + first[1:], ExpiresAt: time.Now().Add(10 * time.Minute)}); err != nil {
		t.Fatalf("Replace returned error: %v", err)
	}
	if _, err := passes.Replace(ctx, &models.PatientPass{PatientID: fixture.patient.ID, CreatedByUserID: fixture.booker.ID, TokenHash: "t" + second[1:], CodeHash: "c" + second[1:], ExpiresAt: time.Now().Add(10 * time.Minute)}); err != nil {
		t.Fatalf("second Replace returned error: %v", err)
	}
	if _, err := passes.GetByTokenHash(ctx, "t"+first[1:]); !errors.Is(err, apperrors.ErrPatientPassInvalid) {
		t.Fatalf("GetByTokenHash of the replaced pass error = %v, want %v", err, apperrors.ErrPatientPassInvalid)
	}
	if pass, err := passes.GetByCodeHash(ctx, "c"+second[1:]); err != nil || pass.PatientID != fixture.patient.ID {
		t.Fatalf("GetByCodeHash = %+v, %v; want the patient's current pass", pass, err)
	}

	if _, err := clinicPatients.Get(ctx, fixture.doctor.ID, fixture.patient.ID); !errors.Is(err, apperrors.ErrPatientNotAtClinic) {
		t.Fatalf("Get before Add error = %v, want %v", err, apperrors.ErrPatientNotAtClinic)
	}
	if err := clinicPatients.Add(ctx, &models.ClinicPatient{DoctorID: fixture.doctor.ID, PatientID: fixture.patient.ID, Source: models.ClinicPatientSourceWalkIn, CreatedByUserID: fixture.booker.ID}); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	if err := clinicPatients.Add(ctx, &models.ClinicPatient{DoctorID: fixture.doctor.ID, PatientID: fixture.patient.ID, Source: models.ClinicPatientSourcePass, CreatedByUserID: fixture.booker.ID}); err != nil {
		t.Fatalf("Add again returned error: %v", err)
	}
	seen, err := clinicPatients.Get(ctx, fixture.doctor.ID, fixture.patient.ID)
	if err != nil || seen.Source != models.ClinicPatientSourceWalkIn {
		t.Fatalf("Get = %+v, %v; want the first source, walk_in, kept", seen, err)
	}
}

func TestAppointmentRepositoryBookedVia(t *testing.T) {
	db := openTestDB(t)
	fixture := newAppointmentFixture(t, db)
	ctx := context.Background()
	startsAt := time.Now().Add(190 * time.Hour).Truncate(time.Hour)

	fromApp := newUnpaidAppointment(t, fixture, startsAt, time.Now().Add(10*time.Minute))
	if fromApp.BookedVia != models.BookedViaApp {
		t.Fatalf("Create without BookedVia = %q, want app", fromApp.BookedVia)
	}

	atDesk, err := fixture.appointments.Create(ctx, &models.Appointment{
		PatientID:       fixture.patient.ID,
		DoctorID:        fixture.doctor.ID,
		BookedByUserID:  fixture.booker.ID,
		StartsAt:        startsAt.Add(time.Hour),
		EndsAt:          startsAt.Add(90 * time.Minute),
		Mode:            models.ConsultationModeInPerson,
		Status:          models.AppointmentStatusScheduled,
		ConsultationFee: 50000,
		SlotDurationMin: 30,
		PaymentStatus:   models.AppointmentPaymentDueAtClinic,
		BookedVia:       models.BookedViaDesk,
	})
	if err != nil {
		t.Fatalf("Create at desk returned error: %v", err)
	}
	found, err := fixture.appointments.GetByID(ctx, atDesk.ID)
	if err != nil || found.BookedVia != models.BookedViaDesk {
		t.Fatalf("GetByID = %+v, %v; want booked via desk", found, err)
	}
}
