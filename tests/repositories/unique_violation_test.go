package repositories_test

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/repositories"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestUserRepositoryCreateReportsWhichFieldClashed(t *testing.T) {
	db := openTestDB(t)
	users, err := repositories.NewUserRepository(db)
	if err != nil {
		t.Fatalf("NewUserRepository returned error: %v", err)
	}
	txManager, err := repositories.NewGORMTransactionManager(db)
	if err != nil {
		t.Fatalf("NewGORMTransactionManager returned error: %v", err)
	}

	existing := newTestUser(t, db)
	if _, err := users.Create(context.Background(), existing); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	tests := []struct {
		name          string
		sameEmail     bool
		sameMobileNo  bool
		inTransaction bool
		wantErr       error
	}{
		{
			name:      "same email",
			sameEmail: true,
			wantErr:   apperrors.ErrEmailAlreadyExists,
		},
		{
			name:         "same mobile number",
			sameMobileNo: true,
			wantErr:      apperrors.ErrMobileAlreadyExists,
		},
		{
			name:          "same email inside a transaction",
			sameEmail:     true,
			inTransaction: true,
			wantErr:       apperrors.ErrEmailAlreadyExists,
		},
	}

	parent := t
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := newTestUser(parent, db)
			if tt.sameEmail {
				user.Email = existing.Email
			}
			if tt.sameMobileNo {
				user.MobileNo = existing.MobileNo
			}

			var err error
			if tt.inTransaction {
				err = txManager.WithinTransaction(context.Background(), func(txCtx context.Context) error {
					_, createErr := users.Create(txCtx, user)
					return createErr
				})
			} else {
				_, err = users.Create(context.Background(), user)
			}

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Create error = %v, want %v", err, tt.wantErr)
			}
			if !errors.Is(err, apperrors.ErrUserAlreadyExists) {
				t.Fatalf("Create error = %v, want it to still match %v", err, apperrors.ErrUserAlreadyExists)
			}
		})
	}
}

func TestDoctorRepositoryCreateReportsWhichRuleClashed(t *testing.T) {
	db := openTestDB(t)
	users, err := repositories.NewUserRepository(db)
	if err != nil {
		t.Fatalf("NewUserRepository returned error: %v", err)
	}
	doctors, err := repositories.NewDoctorRepository(db)
	if err != nil {
		t.Fatalf("NewDoctorRepository returned error: %v", err)
	}

	newDoctorUser := func() *models.User {
		user := newTestUser(t, db)
		created, err := users.Create(context.Background(), user)
		if err != nil {
			t.Fatalf("Create user returned error: %v", err)
		}
		t.Cleanup(func() {
			db.Exec("DELETE FROM doctors WHERE user_id = ?", created.ID)
		})
		return created
	}
	newDoctor := func(userID uint64, registrationNumber string) *models.Doctor {
		return &models.Doctor{
			UserID:              userID,
			FullName:            "Dr. Test",
			RegistrationNumber:  registrationNumber,
			RegistrationCouncil: "Test Council",
			Qualification:       "MBBS",
			SlotDurationMin:     15,
			VerificationStatus:  models.VerificationStatusPending,
			IsAcceptingPatients: true,
		}
	}

	firstUser := newDoctorUser()
	registration := fmt.Sprintf("REG-%d", time.Now().UnixNano())
	if _, err := doctors.Create(context.Background(), newDoctor(firstUser.ID, registration)); err != nil {
		t.Fatalf("Create doctor returned error: %v", err)
	}

	if _, err := doctors.Create(context.Background(), newDoctor(firstUser.ID, registration+"-other")); !errors.Is(err, apperrors.ErrDoctorAlreadyExists) {
		t.Fatalf("Create error = %v, want %v", err, apperrors.ErrDoctorAlreadyExists)
	}

	secondUser := newDoctorUser()
	if _, err := doctors.Create(context.Background(), newDoctor(secondUser.ID, registration)); !errors.Is(err, apperrors.ErrDoctorRegistrationExists) {
		t.Fatalf("Create error = %v, want %v", err, apperrors.ErrDoctorRegistrationExists)
	}
}

func TestSpecialityRepositoryCreateDuplicateSlug(t *testing.T) {
	db := openTestDB(t)
	specialities, err := repositories.NewSpecialityRepository(db)
	if err != nil {
		t.Fatalf("NewSpecialityRepository returned error: %v", err)
	}

	slug := fmt.Sprintf("test-slug-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		db.Exec("DELETE FROM specialities WHERE slug = ?", slug)
	})

	if _, err := specialities.Create(context.Background(), &models.Speciality{Slug: slug, Name: "Test"}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, err := specialities.Create(context.Background(), &models.Speciality{Slug: slug, Name: "Test"}); !errors.Is(err, apperrors.ErrSpecialityAlreadyExists) {
		t.Fatalf("duplicate slug: error = %v, want %v", err, apperrors.ErrSpecialityAlreadyExists)
	}
}
