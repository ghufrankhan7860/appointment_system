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
	"appointment_system/internal/apps/appointment/utils"
)

type staffFixture struct {
	doctors  *fakeDoctorRepo
	staff    *fakeStaffRepo
	invites  *fakeStaffInviteRepo
	attempts *fakeStaffInviteAttemptRepo
	sessions *fakeSessionRepo
	users    *fakeUserRepo
	now      time.Time
}

func newStaffFixture() *staffFixture {
	doctor := clinicDoctor(models.VerificationStatusVerified)
	return &staffFixture{
		doctors:  &fakeDoctorRepo{byID: doctor, byUserID: doctor},
		staff:    &fakeStaffRepo{},
		invites:  &fakeStaffInviteRepo{},
		attempts: &fakeStaffInviteAttemptRepo{},
		sessions: &fakeSessionRepo{},
		users:    &fakeUserRepo{user: &models.User{ID: 40, Name: "Ravi", Role: models.RoleStaff}},
		now:      time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC),
	}
}

func (f *staffFixture) service(t *testing.T) *services.StaffService {
	t.Helper()

	service, err := services.NewStaffService(f.users, f.doctors, f.staff, f.invites, f.attempts, f.sessions, &fakeTxManager{}, func() time.Time { return f.now }, discardLogger)
	if err != nil {
		t.Fatalf("NewStaffService returned error: %v", err)
	}
	return service
}

// invite stores a pending invite for doctor 5 and returns its code.
func (f *staffFixture) invite(code string, expiresAt time.Time) string {
	f.invites.invites = append(f.invites.invites, &models.StaffInvite{
		ID:        uint64(len(f.invites.invites) + 1),
		DoctorID:  5,
		CodeHash:  utils.HashToken(code),
		Status:    models.StaffInviteStatusPending,
		ExpiresAt: expiresAt,
	})
	return code
}

func TestStaffServiceCreateInvite(t *testing.T) {
	tests := []struct {
		name      string
		role      models.Role
		status    models.VerificationStatus
		pending   int
		createErr []error
		wantErr   error
	}{
		{name: "a verified doctor gets a code", role: models.RoleDoctor, status: models.VerificationStatusVerified},
		{name: "a clashing code is drawn again", role: models.RoleDoctor, status: models.VerificationStatusVerified, createErr: []error{apperrors.ErrStaffInviteCodeExists, nil}},
		{name: "a doctor under review", role: models.RoleDoctor, status: models.VerificationStatusPending, wantErr: apperrors.ErrDoctorNotVerified},
		{name: "too many open invites", role: models.RoleDoctor, status: models.VerificationStatusVerified, pending: constant.MaxPendingStaffInvites, wantErr: apperrors.ErrStaffInviteLimitReached},
		{name: "staff cannot invite staff", role: models.RoleStaff, status: models.VerificationStatusVerified, wantErr: apperrors.ErrDoctorRoleRequired},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newStaffFixture()
			fixture.doctors.byUserID.VerificationStatus = tt.status
			fixture.invites.createErr = tt.createErr
			for index := 0; index < tt.pending; index++ {
				fixture.invite("CODE000"+string(rune('A'+index)), fixture.now.Add(time.Hour))
			}

			invite, code, err := fixture.service(t).CreateInvite(context.Background(), 11, tt.role)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("CreateInvite error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("CreateInvite returned error: %v", err)
			}
			if len(code) != constant.PatientInviteCodeLength || invite.CodeHash != utils.HashToken(code) || invite.DoctorID != 5 || !invite.ExpiresAt.Equal(fixture.now.Add(constant.StaffInviteTTL)) {
				t.Fatalf("CreateInvite = %+v with code %q, want doctor 5's invite for 48 hours storing only the code's hash", invite, code)
			}
		})
	}
}

func TestStaffServiceRegister(t *testing.T) {
	tests := []struct {
		name         string
		code         func(f *staffFixture) string
		password     string
		failed       int64
		userErr      error
		wantErr      error
		wantAttempts int64
		wantStatus   models.StaffInviteStatus
	}{
		{name: "a valid code signs up staff for the doctor", code: func(f *staffFixture) string {
			return strings.ToLower(f.invite("K7Q2M9XD", f.now.Add(time.Hour))[:4]) + "-M9XD"
		}, wantStatus: models.StaffInviteStatusRedeemed},
		{name: "an unknown code", code: func(f *staffFixture) string { return "ZZZZZZZZ" }, wantErr: apperrors.ErrStaffInviteInvalid, wantAttempts: 1},
		{name: "an expired code is expired and counted", code: func(f *staffFixture) string { return f.invite("K7Q2M9XD", f.now.Add(-time.Minute)) }, wantErr: apperrors.ErrStaffInviteInvalid, wantAttempts: 1, wantStatus: models.StaffInviteStatusExpired},
		{name: "too many wrong codes from this address", code: func(f *staffFixture) string { return f.invite("K7Q2M9XD", f.now.Add(time.Hour)) }, failed: constant.MaxFailedStaffInviteAttempts, wantErr: apperrors.ErrStaffInviteAttemptsExceeded, wantAttempts: constant.MaxFailedStaffInviteAttempts, wantStatus: models.StaffInviteStatusPending},
		{name: "a code of the wrong length is not counted", code: func(f *staffFixture) string { return "K7Q2" }, wantErr: apperrors.ErrInvalidStaffInput},
		{name: "a short password", code: func(f *staffFixture) string { return f.invite("K7Q2M9XD", f.now.Add(time.Hour)) }, password: "short", wantErr: apperrors.ErrInvalidRegisterInput, wantStatus: models.StaffInviteStatusPending},
		{name: "an email already registered keeps the invite", code: func(f *staffFixture) string { return f.invite("K7Q2M9XD", f.now.Add(time.Hour)) }, userErr: apperrors.ErrEmailAlreadyExists, wantErr: apperrors.ErrUserAlreadyExists, wantStatus: models.StaffInviteStatusPending},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newStaffFixture()
			fixture.users.err = tt.userErr
			if tt.failed > 0 {
				fixture.attempts.failed = map[string]int64{"10.0.0.1": tt.failed}
			}
			code := tt.code(fixture)
			password := tt.password
			if password == "" {
				password = "staff-password"
			}

			created, err := fixture.service(t).Register(context.Background(), &models.User{Name: "Ravi", Email: "ravi@example.com", MobileNo: "9876543210", Role: models.RoleAdmin}, password, code, "10.0.0.1")
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("Register error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("Register returned error: %v", err)
				}
				link, linkErr := fixture.staff.GetActiveByUser(context.Background(), created.ID)
				if linkErr != nil || link.DoctorID != 5 || created.Role != models.RoleStaff {
					t.Fatalf("Register made %+v with link %+v, %v; want a staff account working for doctor 5", created, link, linkErr)
				}
			}
			if got := fixture.attempts.failed["10.0.0.1"]; got != tt.wantAttempts {
				t.Fatalf("Register counted %d wrong codes, want %d", got, tt.wantAttempts)
			}
			if tt.wantStatus != "" && fixture.invites.invites[0].Status != tt.wantStatus {
				t.Fatalf("Register left the invite %q, want %q", fixture.invites.invites[0].Status, tt.wantStatus)
			}
		})
	}
}

func TestStaffServiceRemoveAndJoin(t *testing.T) {
	t.Run("removing staff signs them out everywhere", func(t *testing.T) {
		fixture := newStaffFixture()
		fixture.staff.active = map[uint64]*models.DoctorStaff{40: {DoctorID: 5, UserID: 40}}

		if err := fixture.service(t).Remove(context.Background(), 11, models.RoleDoctor, 40); err != nil {
			t.Fatalf("Remove returned error: %v", err)
		}
		if len(fixture.staff.removed) != 1 || fixture.sessions.revokedUserID != 40 || fixture.sessions.exceptID != 0 {
			t.Fatalf("Remove removed %v and revoked sessions of %d, want staff 40 removed and signed out", fixture.staff.removed, fixture.sessions.revokedUserID)
		}
	})

	t.Run("another doctor's staff", func(t *testing.T) {
		fixture := newStaffFixture()
		fixture.staff.active = map[uint64]*models.DoctorStaff{40: {DoctorID: 6, UserID: 40}}

		if err := fixture.service(t).Remove(context.Background(), 11, models.RoleDoctor, 40); !errors.Is(err, apperrors.ErrStaffNotFound) {
			t.Fatalf("Remove error = %v, want %v", err, apperrors.ErrStaffNotFound)
		}
		if fixture.sessions.revokedUserID != 0 {
			t.Fatalf("Remove revoked the sessions of %d, want none", fixture.sessions.revokedUserID)
		}
	})

	t.Run("removed staff join another doctor", func(t *testing.T) {
		fixture := newStaffFixture()
		code := fixture.invite("K7Q2M9XD", fixture.now.Add(time.Hour))

		link, err := fixture.service(t).Join(context.Background(), 40, models.RoleStaff, code, "10.0.0.1")
		if err != nil || link.DoctorID != 5 || len(fixture.invites.redeemed) != 1 || fixture.invites.redeemed[0] != 40 {
			t.Fatalf("Join = %+v, %v; want staff 40 working for doctor 5 and the invite redeemed by them", link, err)
		}
	})

	t.Run("staff already working for a doctor", func(t *testing.T) {
		fixture := newStaffFixture()
		fixture.staff.active = map[uint64]*models.DoctorStaff{40: {DoctorID: 6, UserID: 40}}
		code := fixture.invite("K7Q2M9XD", fixture.now.Add(time.Hour))

		if _, err := fixture.service(t).Join(context.Background(), 40, models.RoleStaff, code, "10.0.0.1"); !errors.Is(err, apperrors.ErrStaffAlreadyLinked) {
			t.Fatalf("Join error = %v, want %v", err, apperrors.ErrStaffAlreadyLinked)
		}
		if fixture.invites.invites[0].Status != models.StaffInviteStatusPending {
			t.Fatalf("Join left the invite %q, want it still pending", fixture.invites.invites[0].Status)
		}
	})

	t.Run("a patient account cannot join", func(t *testing.T) {
		fixture := newStaffFixture()
		code := fixture.invite("K7Q2M9XD", fixture.now.Add(time.Hour))

		if _, err := fixture.service(t).Join(context.Background(), 7, models.RolePatient, code, "10.0.0.1"); !errors.Is(err, apperrors.ErrStaffNotLinked) {
			t.Fatalf("Join error = %v, want %v", err, apperrors.ErrStaffNotLinked)
		}
	})
}

func TestClinicActorDeskDoctor(t *testing.T) {
	doctor := clinicDoctor(models.VerificationStatusVerified)
	staff := &fakeStaffRepo{active: map[uint64]*models.DoctorStaff{40: {DoctorID: 5, UserID: 40}}}
	actor, err := services.NewClinicActor(&fakeDoctorRepo{byID: doctor, byUserID: doctor}, staff, &fakeClinicPatientRepo{})
	if err != nil {
		t.Fatalf("NewClinicActor returned error: %v", err)
	}

	tests := []struct {
		name    string
		userID  uint64
		role    models.Role
		wantErr error
	}{
		{name: "a doctor runs their own desk", userID: 11, role: models.RoleDoctor},
		{name: "staff run their doctor's desk", userID: 40, role: models.RoleStaff},
		{name: "staff no doctor has", userID: 41, role: models.RoleStaff, wantErr: apperrors.ErrStaffNotLinked},
		{name: "a patient", userID: 7, role: models.RolePatient, wantErr: apperrors.ErrDoctorRoleRequired},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found, err := actor.DeskDoctor(context.Background(), tt.userID, tt.role)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) || !errors.Is(err, apperrors.ErrDoctorRoleRequired) {
					t.Fatalf("DeskDoctor error = %v, want %v (and a doctor role error)", err, tt.wantErr)
				}
				return
			}
			if err != nil || found.ID != 5 {
				t.Fatalf("DeskDoctor = %+v, %v; want doctor 5", found, err)
			}
		})
	}
}
