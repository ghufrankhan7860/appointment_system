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

func knownSpecialities() *fakeSpecialityRepo {
	return &fakeSpecialityRepo{known: map[string]*models.Speciality{
		"general-medicine": {ID: 1, Slug: "general-medicine", Name: "General Medicine"},
		"diabetology":      {ID: 2, Slug: "diabetology", Name: "Diabetology"},
	}}
}

// newTestAvailability is an availability engine with no rules, for doctor
// service tests that never search by date.
func newTestAvailability(t *testing.T) *services.AvailabilityService {
	t.Helper()

	return newTestAvailabilityService(t, nil, &fakeAvailabilityRepo{}, time.Now())
}

func validDoctorInput() *models.Doctor {
	return &models.Doctor{
		FullName:            "  Dr. Asha Verma ",
		RegistrationNumber:  "DMC/R/12345",
		RegistrationCouncil: "Delhi Medical Council",
		RegistrationYear:    2012,
		Qualification:       "MBBS, MD (Medicine)",
		YearsOfExperience:   11,
		ConsultationFee:     60000,
		City:                " New Delhi ",
		ConsultationModes:   []models.ConsultationMode{models.ConsultationModeInPerson, models.ConsultationModeVideo, models.ConsultationModeVideo},
		Tags:                []models.DoctorTag{{Tag: " Diabetes "}, {Tag: "diabetes"}, {Tag: "Hindi"}, {Tag: ""}},
	}
}

func doctorWithFullName(fullName string) *models.Doctor {
	doctor := validDoctorInput()
	doctor.FullName = fullName

	return doctor
}

func doctorWithModes(modes ...models.ConsultationMode) *models.Doctor {
	doctor := validDoctorInput()
	doctor.ConsultationModes = modes

	return doctor
}

func doctorWithRegistrationYear(year int) *models.Doctor {
	doctor := validDoctorInput()
	doctor.RegistrationYear = year

	return doctor
}

func doctorWithFee(fee int) *models.Doctor {
	doctor := validDoctorInput()
	doctor.ConsultationFee = fee

	return doctor
}

func TestNewDoctorServiceValidation(t *testing.T) {
	tests := []struct {
		name          string
		doctors       services.DoctorRepository
		specialities  services.SpecialityRepository
		verifications services.DoctorVerificationRepository
		availability  *services.AvailabilityService
		tx            services.TransactionManager
		wantErr       error
	}{
		{name: "nil doctors", specialities: &fakeSpecialityRepo{}, verifications: &fakeDoctorVerificationRepo{}, availability: newTestAvailability(t), tx: &fakeTxManager{}, wantErr: apperrors.ErrNilDoctorRepository},
		{name: "nil specialities", doctors: &fakeDoctorRepo{}, verifications: &fakeDoctorVerificationRepo{}, availability: newTestAvailability(t), tx: &fakeTxManager{}, wantErr: apperrors.ErrNilSpecialityRepository},
		{name: "nil verifications", doctors: &fakeDoctorRepo{}, specialities: &fakeSpecialityRepo{}, availability: newTestAvailability(t), tx: &fakeTxManager{}, wantErr: apperrors.ErrNilDoctorVerificationRepository},
		{name: "nil availability", doctors: &fakeDoctorRepo{}, specialities: &fakeSpecialityRepo{}, verifications: &fakeDoctorVerificationRepo{}, tx: &fakeTxManager{}, wantErr: apperrors.ErrNilAvailabilityService},
		{name: "nil tx", doctors: &fakeDoctorRepo{}, specialities: &fakeSpecialityRepo{}, verifications: &fakeDoctorVerificationRepo{}, availability: newTestAvailability(t), wantErr: apperrors.ErrNilTransactionManager},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := services.NewDoctorService(tt.doctors, tt.specialities, tt.verifications, tt.availability, tt.tx, discardLogger); !errors.Is(err, tt.wantErr) {
				t.Fatalf("NewDoctorService error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestDoctorServiceCreate(t *testing.T) {
	t.Run("success normalizes and starts pending", func(t *testing.T) {
		doctors := &fakeDoctorRepo{}
		tx := &fakeTxManager{}
		service, err := services.NewDoctorService(doctors, knownSpecialities(), &fakeDoctorVerificationRepo{}, newTestAvailability(t), tx, discardLogger)
		if err != nil {
			t.Fatalf("NewDoctorService returned error: %v", err)
		}

		created, err := service.Create(context.Background(), 11, models.RoleDoctor, validDoctorInput(), []string{"Diabetology", "general-medicine", "diabetology"})
		if err != nil {
			t.Fatalf("Create error = %v", err)
		}
		if !tx.called {
			t.Fatalf("Create did not run inside transaction")
		}
		if created.UserID != 11 || created.VerificationStatus != models.VerificationStatusPending || !created.IsAcceptingPatients {
			t.Fatalf("created doctor = %+v", created)
		}
		if created.FullName != "Dr. Asha Verma" || created.City != "New Delhi" || created.SlotDurationMin != 15 {
			t.Fatalf("Create did not normalize doctor: %+v", created)
		}
		if len(created.Specialities) != 2 || created.Specialities[0].Slug != "diabetology" || !created.Specialities[0].IsPrimary || created.Specialities[1].IsPrimary {
			t.Fatalf("specialities = %+v", created.Specialities)
		}
		if len(created.Tags) != 2 || created.Tags[0].Tag != "diabetes" || created.Tags[1].Tag != "hindi" {
			t.Fatalf("tags = %+v", created.Tags)
		}
		if len(created.ConsultationModes) != 2 {
			t.Fatalf("modes = %+v", created.ConsultationModes)
		}
	})

	tests := []struct {
		name         string
		userID       uint64
		role         models.Role
		doctors      *fakeDoctorRepo
		specialities *fakeSpecialityRepo
		input        *models.Doctor
		slugs        []string
		wantErr      error
	}{
		{name: "patient role is rejected", userID: 11, role: models.RolePatient, wantErr: apperrors.ErrDoctorRoleRequired},
		{name: "admin role is rejected", userID: 11, role: models.RoleAdmin, wantErr: apperrors.ErrDoctorRoleRequired},
		{name: "zero user is invalid", userID: 0, role: models.RoleDoctor, wantErr: apperrors.ErrInvalidDoctorInput},
		{name: "existing profile conflicts", userID: 11, role: models.RoleDoctor, doctors: &fakeDoctorRepo{byUserID: &models.Doctor{ID: 5}}, wantErr: apperrors.ErrDoctorAlreadyExists},
		{name: "unknown speciality", userID: 11, role: models.RoleDoctor, slugs: []string{"astrology"}, wantErr: apperrors.ErrSpecialityNotFound},
		{name: "no speciality", userID: 11, role: models.RoleDoctor, slugs: []string{" "}, wantErr: apperrors.ErrInvalidDoctorInput},
		{name: "invalid mode", userID: 11, role: models.RoleDoctor, input: doctorWithModes("teleport"), wantErr: apperrors.ErrInvalidDoctorInput},
		{name: "missing name", userID: 11, role: models.RoleDoctor, input: doctorWithFullName(" "), wantErr: apperrors.ErrInvalidDoctorInput},
		{name: "future registration year", userID: 11, role: models.RoleDoctor, input: doctorWithRegistrationYear(time.Now().Year() + 1), wantErr: apperrors.ErrInvalidDoctorInput},
		{name: "negative fee", userID: 11, role: models.RoleDoctor, input: doctorWithFee(-1), wantErr: apperrors.ErrInvalidDoctorInput},
		{name: "duplicate registration", userID: 11, role: models.RoleDoctor, doctors: &fakeDoctorRepo{createErr: apperrors.ErrDoctorRegistrationExists}, wantErr: apperrors.ErrDoctorRegistrationExists},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doctors := tt.doctors
			if doctors == nil {
				doctors = &fakeDoctorRepo{}
			}
			specialities := tt.specialities
			if specialities == nil {
				specialities = knownSpecialities()
			}
			slugs := tt.slugs
			if slugs == nil {
				slugs = []string{"general-medicine"}
			}
			input := tt.input
			if input == nil {
				input = validDoctorInput()
			}

			service, err := services.NewDoctorService(doctors, specialities, &fakeDoctorVerificationRepo{}, newTestAvailability(t), &fakeTxManager{}, discardLogger)
			if err != nil {
				t.Fatalf("NewDoctorService returned error: %v", err)
			}

			if _, err := service.Create(context.Background(), tt.userID, tt.role, input, slugs); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Create error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestDoctorServiceUpdate(t *testing.T) {
	verifiedAt := time.Now().Add(-24 * time.Hour)
	accepting := false

	tests := []struct {
		name             string
		existing         *models.Doctor
		accepting        *bool
		wantStatus       models.VerificationStatus
		wantAudit        bool
		wantAccepting    bool
		wantVerifiedAtOK bool
	}{
		{
			name:          "rejected profile is resubmitted as pending",
			existing:      &models.Doctor{ID: 5, UserID: 11, RegistrationNumber: "DMC/R/12345", RegistrationCouncil: "Delhi Medical Council", VerificationStatus: models.VerificationStatusRejected, IsAcceptingPatients: true},
			wantStatus:    models.VerificationStatusPending,
			wantAudit:     true,
			wantAccepting: true,
		},
		{
			name:          "verified doctor changing registration goes back to pending",
			existing:      &models.Doctor{ID: 5, UserID: 11, RegistrationNumber: "OLD/1", RegistrationCouncil: "Delhi Medical Council", VerificationStatus: models.VerificationStatusVerified, VerifiedAt: &verifiedAt, IsAcceptingPatients: true},
			wantStatus:    models.VerificationStatusPending,
			wantAudit:     true,
			wantAccepting: true,
		},
		{
			name:             "verified doctor editing other fields stays verified",
			existing:         &models.Doctor{ID: 5, UserID: 11, RegistrationNumber: "DMC/R/12345", RegistrationCouncil: "Delhi Medical Council", VerificationStatus: models.VerificationStatusVerified, VerifiedAt: &verifiedAt, IsAcceptingPatients: true},
			wantStatus:       models.VerificationStatusVerified,
			wantAudit:        false,
			wantAccepting:    true,
			wantVerifiedAtOK: true,
		},
		{
			name:          "suspended doctor cannot unsuspend by editing",
			existing:      &models.Doctor{ID: 5, UserID: 11, RegistrationNumber: "OLD/1", RegistrationCouncil: "Delhi Medical Council", VerificationStatus: models.VerificationStatusSuspended, IsAcceptingPatients: true},
			wantStatus:    models.VerificationStatusSuspended,
			wantAudit:     false,
			wantAccepting: true,
		},
		{
			name:          "accepting patients can be turned off",
			existing:      &models.Doctor{ID: 5, UserID: 11, RegistrationNumber: "DMC/R/12345", RegistrationCouncil: "Delhi Medical Council", VerificationStatus: models.VerificationStatusPending, IsAcceptingPatients: true},
			accepting:     &accepting,
			wantStatus:    models.VerificationStatusPending,
			wantAudit:     false,
			wantAccepting: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verifications := &fakeDoctorVerificationRepo{}
			service, err := services.NewDoctorService(&fakeDoctorRepo{byUserID: tt.existing}, knownSpecialities(), verifications, newTestAvailability(t), &fakeTxManager{}, discardLogger)
			if err != nil {
				t.Fatalf("NewDoctorService returned error: %v", err)
			}

			updated, err := service.Update(context.Background(), 11, models.RoleDoctor, validDoctorInput(), []string{"general-medicine"}, tt.accepting)
			if err != nil {
				t.Fatalf("Update error = %v", err)
			}
			if updated.VerificationStatus != tt.wantStatus {
				t.Fatalf("Update status = %s, want %s", updated.VerificationStatus, tt.wantStatus)
			}
			if updated.IsAcceptingPatients != tt.wantAccepting {
				t.Fatalf("Update accepting = %v, want %v", updated.IsAcceptingPatients, tt.wantAccepting)
			}
			if tt.wantVerifiedAtOK && updated.VerifiedAt == nil {
				t.Fatalf("verified_at was cleared for a still verified doctor")
			}
			if tt.wantStatus == models.VerificationStatusPending && updated.VerifiedAt != nil {
				t.Fatalf("verified_at should be cleared when returning to pending")
			}
			if (len(verifications.created) == 1) != tt.wantAudit {
				t.Fatalf("audit rows = %d, want audit %v", len(verifications.created), tt.wantAudit)
			}
			if tt.wantAudit && verifications.created[0].ReviewedByUserID != 11 {
				t.Fatalf("audit actor = %d, want the doctor", verifications.created[0].ReviewedByUserID)
			}
		})
	}

	t.Run("missing profile", func(t *testing.T) {
		service, _ := services.NewDoctorService(&fakeDoctorRepo{}, knownSpecialities(), &fakeDoctorVerificationRepo{}, newTestAvailability(t), &fakeTxManager{}, discardLogger)
		if _, err := service.Update(context.Background(), 11, models.RoleDoctor, validDoctorInput(), []string{"general-medicine"}, nil); !errors.Is(err, apperrors.ErrDoctorNotFound) {
			t.Fatalf("Update error = %v, want %v", err, apperrors.ErrDoctorNotFound)
		}
	})
}

func TestDoctorServiceGetByID(t *testing.T) {
	tests := []struct {
		name    string
		doctor  *models.Doctor
		wantErr error
	}{
		{name: "verified doctor is visible", doctor: &models.Doctor{ID: 5, VerificationStatus: models.VerificationStatusVerified}},
		{name: "pending doctor is hidden", doctor: &models.Doctor{ID: 5, VerificationStatus: models.VerificationStatusPending}, wantErr: apperrors.ErrDoctorNotFound},
		{name: "suspended doctor is hidden", doctor: &models.Doctor{ID: 5, VerificationStatus: models.VerificationStatusSuspended}, wantErr: apperrors.ErrDoctorNotFound},
		{name: "missing doctor", wantErr: apperrors.ErrDoctorNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, _ := services.NewDoctorService(&fakeDoctorRepo{byID: tt.doctor}, knownSpecialities(), &fakeDoctorVerificationRepo{}, newTestAvailability(t), &fakeTxManager{}, discardLogger)
			if _, err := service.GetByID(context.Background(), 5); !errors.Is(err, tt.wantErr) {
				t.Fatalf("GetByID error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestDoctorServiceList(t *testing.T) {
	t.Run("normalizes filters and clamps pagination", func(t *testing.T) {
		doctors := &fakeDoctorRepo{}
		service, _ := services.NewDoctorService(doctors, knownSpecialities(), &fakeDoctorVerificationRepo{}, newTestAvailability(t), &fakeTxManager{}, discardLogger)

		if _, err := service.List(context.Background(), models.DoctorSearch{
			Query:        "  thyroid ",
			Specialities: []string{"Cardiology", "cardiology"},
			Tags:         []string{" Hindi"},
			Limit:        5000,
			Offset:       -3,
		}); err != nil {
			t.Fatalf("List error = %v", err)
		}
		search := doctors.searchInput
		if search.Query != "thyroid" || len(search.Specialities) != 1 || search.Specialities[0] != "cardiology" || search.Tags[0] != "hindi" {
			t.Fatalf("search = %+v", search)
		}
		if search.Limit != constant.MaxListLimit || search.Offset != 0 {
			t.Fatalf("pagination = %d/%d", search.Limit, search.Offset)
		}
	})

	t.Run("invalid mode", func(t *testing.T) {
		service, _ := services.NewDoctorService(&fakeDoctorRepo{}, knownSpecialities(), &fakeDoctorVerificationRepo{}, newTestAvailability(t), &fakeTxManager{}, discardLogger)
		if _, err := service.List(context.Background(), models.DoctorSearch{Mode: "teleport"}); !errors.Is(err, apperrors.ErrInvalidDoctorInput) {
			t.Fatalf("List error = %v, want %v", err, apperrors.ErrInvalidDoctorInput)
		}
	})
}

func TestDoctorServiceCreateSpeciality(t *testing.T) {
	tests := []struct {
		name    string
		slug    string
		repo    *fakeSpecialityRepo
		wantErr error
	}{
		{name: "valid slug", slug: " Nephrology ", repo: &fakeSpecialityRepo{}},
		{name: "slug with spaces", slug: "kidney care", repo: &fakeSpecialityRepo{}, wantErr: apperrors.ErrInvalidDoctorInput},
		{name: "duplicate slug", slug: "cardiology", repo: &fakeSpecialityRepo{err: apperrors.ErrSpecialityAlreadyExists}, wantErr: apperrors.ErrSpecialityAlreadyExists},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, _ := services.NewDoctorService(&fakeDoctorRepo{}, tt.repo, &fakeDoctorVerificationRepo{}, newTestAvailability(t), &fakeTxManager{}, discardLogger)
			if _, err := service.CreateSpeciality(context.Background(), tt.slug, "Name"); !errors.Is(err, tt.wantErr) {
				t.Fatalf("CreateSpeciality error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
