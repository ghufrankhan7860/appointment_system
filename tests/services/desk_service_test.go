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
	"appointment_system/internal/apps/appointment/services"
)

type deskFixture struct {
	doctors        *fakeDoctorRepo
	staff          *fakeStaffRepo
	clinicPatients *fakeClinicPatientRepo
	patients       *fakePatientRepo
	userPatients   *fakeUserPatientRepo
	passes         *fakePatientPassRepo
	attempts       *fakePatientPassAttemptRepo
	invites        *fakePatientInviteRepo
	now            time.Time
}

func newDeskFixture() *deskFixture {
	doctor := clinicDoctor(models.VerificationStatusVerified)
	return &deskFixture{
		doctors:        &fakeDoctorRepo{byID: doctor, byUserID: doctor},
		staff:          &fakeStaffRepo{active: map[uint64]*models.DoctorStaff{40: {DoctorID: 5, UserID: 40}}},
		clinicPatients: &fakeClinicPatientRepo{},
		patients:       &fakePatientRepo{getResult: &models.Patient{ID: 3, Name: "Asha Verma"}, createResult: &models.Patient{ID: 9, Name: "Walk In"}},
		userPatients:   &fakeUserPatientRepo{activeResult: patientLink(models.AccessLevelFull)},
		passes:         &fakePatientPassRepo{},
		attempts:       &fakePatientPassAttemptRepo{},
		invites:        &fakePatientInviteRepo{},
		now:            time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC),
	}
}

func (f *deskFixture) clinic(t *testing.T) *services.ClinicActor {
	t.Helper()

	actor, err := services.NewClinicActor(f.doctors, f.staff, f.clinicPatients)
	if err != nil {
		t.Fatalf("NewClinicActor returned error: %v", err)
	}
	return actor
}

func (f *deskFixture) service(t *testing.T) *services.DeskService {
	t.Helper()

	clock := func() time.Time { return f.now }
	invites, err := services.NewPatientInviteService(f.invites, &fakeInviteAttemptRepo{}, f.userPatients, &fakeUserPatientHistoryRepo{}, f.patients, &fakeOutboxRepo{}, &fakeTxManager{}, clock, discardLogger)
	if err != nil {
		t.Fatalf("NewPatientInviteService returned error: %v", err)
	}
	service, err := services.NewDeskService(f.clinic(t), f.patients, f.userPatients, f.passes, f.attempts, f.clinicPatients, invites, &fakeTxManager{}, clock, discardLogger)
	if err != nil {
		t.Fatalf("NewDeskService returned error: %v", err)
	}
	return service
}

func TestDeskServiceIssuePass(t *testing.T) {
	tests := []struct {
		name    string
		link    *models.UserPatient
		wantErr error
	}{
		{name: "full access gets a pass", link: patientLink(models.AccessLevelFull)},
		{name: "appointments_only access gets a pass", link: patientLink(models.AccessLevelAppointmentsOnly)},
		{name: "read_only access cannot book, so no pass", link: patientLink(models.AccessLevelReadOnly), wantErr: apperrors.ErrPatientForbidden},
		{name: "no link to the patient", wantErr: apperrors.ErrPatientNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newDeskFixture()
			fixture.userPatients = &fakeUserPatientRepo{activeResult: tt.link}
			if tt.link == nil {
				fixture.userPatients.activeErr = apperrors.ErrUserPatientNotFound
			}

			pass, token, code, err := fixture.service(t).IssuePass(context.Background(), 7, 3)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("IssuePass error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("IssuePass returned error: %v", err)
			}
			if len(token) != 22 || len(code) != constant.PatientInviteCodeLength || !pass.ExpiresAt.Equal(fixture.now.Add(constant.PatientPassTTL)) {
				t.Fatalf("IssuePass = %+v, %q, %q; want a 22-character token, an 8-character code and 10 minutes", pass, token, code)
			}
			if pass.TokenHash == token || pass.CodeHash == code {
				t.Fatalf("IssuePass stored the token or code in clear: %+v", pass)
			}
		})
	}
}

func TestDeskServiceLookupPass(t *testing.T) {
	type issued struct{ token, code string }
	tests := []struct {
		name       string
		userID     uint64
		role       models.Role
		value      func(pass issued) string
		later      time.Duration
		failed     int64
		reissue    bool
		wantErr    error
		wantFailed int64
	}{
		{name: "staff scan the token", userID: 40, role: models.RoleStaff, value: func(pass issued) string { return pass.token }},
		{name: "the doctor types the code", userID: 11, role: models.RoleDoctor, value: func(pass issued) string {
			return strings.ToLower(pass.code[:4]) + "-" + pass.code[4:]
		}},
		{name: "an expired pass", userID: 40, role: models.RoleStaff, value: func(pass issued) string { return pass.token }, later: constant.PatientPassTTL, wantErr: apperrors.ErrPatientPassInvalid, wantFailed: 1},
		{name: "an older pass stops working", userID: 40, role: models.RoleStaff, value: func(pass issued) string { return pass.token }, reissue: true, wantErr: apperrors.ErrPatientPassInvalid, wantFailed: 1},
		{name: "a wrong code", userID: 40, role: models.RoleStaff, value: func(pass issued) string { return "ZZZZZZZZ" }, wantErr: apperrors.ErrPatientPassInvalid, wantFailed: 1},
		{name: "too many wrong codes", userID: 40, role: models.RoleStaff, value: func(pass issued) string { return pass.token }, failed: constant.MaxFailedPatientPassAttempts, wantErr: apperrors.ErrPatientPassAttemptsExceeded, wantFailed: constant.MaxFailedPatientPassAttempts},
		{name: "a patient cannot use the desk", userID: 7, role: models.RolePatient, value: func(pass issued) string { return pass.token }, wantErr: apperrors.ErrDoctorRoleRequired},
		{name: "staff no doctor has", userID: 41, role: models.RoleStaff, value: func(pass issued) string { return pass.token }, wantErr: apperrors.ErrStaffNotLinked},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newDeskFixture()
			service := fixture.service(t)
			_, token, code, err := service.IssuePass(context.Background(), 7, 3)
			if err != nil {
				t.Fatalf("IssuePass returned error: %v", err)
			}
			if tt.reissue {
				if _, _, _, err := service.IssuePass(context.Background(), 7, 3); err != nil {
					t.Fatalf("IssuePass again returned error: %v", err)
				}
			}
			if tt.failed > 0 {
				fixture.attempts.failed = map[uint64]int64{tt.userID: tt.failed}
			}
			fixture.now = fixture.now.Add(tt.later)

			patient, err := service.LookupPass(context.Background(), tt.userID, tt.role, tt.value(issued{token: token, code: code}))
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("LookupPass error = %v, want %v", err, tt.wantErr)
				}
				if got := fixture.attempts.failed[tt.userID]; got != tt.wantFailed {
					t.Fatalf("LookupPass counted %d wrong codes, want %d", got, tt.wantFailed)
				}
				if _, err := fixture.clinicPatients.Get(context.Background(), 5, 3); err == nil {
					t.Fatal("LookupPass let the desk book for the patient after an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("LookupPass returned error: %v", err)
			}
			seen, err := fixture.clinicPatients.Get(context.Background(), 5, 3)
			if patient.ID != 3 || err != nil || seen.Source != models.ClinicPatientSourcePass || seen.CreatedByUserID != tt.userID {
				t.Fatalf("LookupPass = %+v with clinic record %+v, %v; want patient 3 now known to doctor 5's desk by pass", patient, seen, err)
			}
		})
	}
}

func TestDeskServiceWalkIn(t *testing.T) {
	walkIn := func() *models.Patient {
		return &models.Patient{Name: " Walk In ", Gender: models.GenderFemale, BloodType: models.BloodTypeOPlus, DateOfBirth: time.Date(1980, 1, 2, 0, 0, 0, 0, time.UTC), EmergencyContact: "98765 43210"}
	}

	t.Run("staff add a walk-in the desk can book", func(t *testing.T) {
		fixture := newDeskFixture()

		created, err := fixture.service(t).CreateWalkIn(context.Background(), 40, models.RoleStaff, walkIn())
		if err != nil {
			t.Fatalf("CreateWalkIn returned error: %v", err)
		}
		seen, err := fixture.clinicPatients.Get(context.Background(), 5, created.ID)
		if err != nil || seen.Source != models.ClinicPatientSourceWalkIn {
			t.Fatalf("CreateWalkIn clinic record = %+v, %v; want a walk-in of doctor 5", seen, err)
		}
		if fixture.userPatients.createCalled {
			t.Fatal("CreateWalkIn linked the patient to the staff account, want nobody holding them")
		}
	})

	t.Run("a walk-in needs the patient details", func(t *testing.T) {
		fixture := newDeskFixture()
		patient := walkIn()
		patient.Name = " "

		if _, err := fixture.service(t).CreateWalkIn(context.Background(), 40, models.RoleStaff, patient); !errors.Is(err, apperrors.ErrInvalidPatientInput) {
			t.Fatalf("CreateWalkIn error = %v, want %v", err, apperrors.ErrInvalidPatientInput)
		}
	})

	tests := []struct {
		name    string
		source  models.ClinicPatientSource
		doctor  uint64
		links   []*models.UserPatient
		wantErr error
	}{
		{name: "a claim code for the desk's walk-in", source: models.ClinicPatientSourceWalkIn, doctor: 5},
		{name: "a patient who showed their pass already uses the app", source: models.ClinicPatientSourcePass, doctor: 5, wantErr: apperrors.ErrPatientAlreadyClaimed},
		{name: "a walk-in someone already took over", source: models.ClinicPatientSourceWalkIn, doctor: 5, links: []*models.UserPatient{patientLink(models.AccessLevelFull)}, wantErr: apperrors.ErrPatientAlreadyClaimed},
		{name: "another clinic's walk-in", source: models.ClinicPatientSourceWalkIn, doctor: 6, wantErr: apperrors.ErrPatientNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newDeskFixture()
			fixture.userPatients.patientLinks = tt.links
			if err := fixture.clinicPatients.Add(context.Background(), &models.ClinicPatient{DoctorID: tt.doctor, PatientID: 9, Source: tt.source}); err != nil {
				t.Fatalf("Add returned error: %v", err)
			}

			invite, code, err := fixture.service(t).WalkInClaimCode(context.Background(), 40, models.RoleStaff, 9, models.RelationshipChild)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("WalkInClaimCode error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("WalkInClaimCode returned error: %v", err)
			}
			if code == "" || invite.PatientID != 9 || invite.AccessLevel != models.AccessLevelFull || invite.PatientRelationship != models.RelationshipChild || invite.CreatedByUserID != 40 {
				t.Fatalf("WalkInClaimCode = %+v, %q; want a full-access child invite for patient 9 made by staff 40", invite, code)
			}
		})
	}
}

func TestAppointmentServiceBookAtDesk(t *testing.T) {
	location := clinicLocation(t)
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, location)
	nineAM := time.Date(2026, 10, 26, 9, 0, 0, 0, location)

	tests := []struct {
		name        string
		userID      uint64
		role        models.Role
		fee         int
		seenAtDesk  bool
		treated     bool
		wantErr     error
		wantPayment models.AppointmentPaymentStatus
	}{
		{name: "staff book a patient who showed their pass", userID: 40, role: models.RoleStaff, fee: 40000, seenAtDesk: true, wantPayment: models.AppointmentPaymentDueAtClinic},
		{name: "the doctor books a patient already in their list", userID: 11, role: models.RoleDoctor, fee: 40000, treated: true, wantPayment: models.AppointmentPaymentDueAtClinic},
		{name: "no fee is waived", userID: 40, role: models.RoleStaff, seenAtDesk: true, wantPayment: models.AppointmentPaymentWaived},
		{name: "a patient the desk has never seen", userID: 40, role: models.RoleStaff, fee: 40000, wantErr: apperrors.ErrPatientNotFound},
		{name: "staff no doctor has", userID: 41, role: models.RoleStaff, fee: 40000, seenAtDesk: true, wantErr: apperrors.ErrStaffNotLinked},
		{name: "a patient account cannot book at the desk", userID: 7, role: models.RolePatient, fee: 40000, seenAtDesk: true, wantErr: apperrors.ErrDoctorRoleRequired},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newDeskFixture()
			fixture.doctors.byID.ConsultationFee = tt.fee
			if tt.seenAtDesk {
				if err := fixture.clinicPatients.Add(context.Background(), &models.ClinicPatient{DoctorID: 5, PatientID: 3, Source: models.ClinicPatientSourcePass}); err != nil {
					t.Fatalf("Add returned error: %v", err)
				}
			}
			appointments := &fakeAppointmentRepo{treated: map[uint64]bool{3: tt.treated}}
			service, _ := newTestServicesWithClinic(t, []models.PaymentMethod{models.PaymentMethodRazorpay}, &fakeFileStorage{}, fixture.clinic(t), fixture.doctors, appointments, &fakeOutboxRepo{}, &fakeUserPatientRepo{activeErr: apperrors.ErrUserPatientNotFound}, &fakePaymentRepo{}, &fakeGateway{}, now)

			_, err := service.BookAtDesk(context.Background(), tt.userID, tt.role, &models.Appointment{PatientID: 3, StartsAt: nineAM, Mode: models.ConsultationModeInPerson})
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("BookAtDesk error = %v, want %v", err, tt.wantErr)
				}
				if appointments.createInput != nil {
					t.Fatalf("BookAtDesk created %+v after an error", appointments.createInput)
				}
				return
			}
			if err != nil {
				t.Fatalf("BookAtDesk returned error: %v", err)
			}
			created := appointments.createInput
			if created.DoctorID != 5 || created.BookedByUserID != tt.userID || created.BookedVia != models.BookedViaDesk ||
				created.PaymentStatus != tt.wantPayment || created.PaymentDueAt != nil {
				t.Fatalf("BookAtDesk created %+v, want doctor 5 booked by %d at the desk, %q with no hold", created, tt.userID, tt.wantPayment)
			}
		})
	}
}
