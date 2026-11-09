package services_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
)

func validTestPatient() *models.Patient {
	return &models.Patient{
		Name:             "Asha Sharma",
		Gender:           models.GenderFemale,
		DateOfBirth:      time.Now().UTC().AddDate(-30, 0, 0),
		BloodType:        models.BloodTypeOPlus,
		EmergencyContact: "9999999999",
	}
}

func TestNewPatientServiceValidation(t *testing.T) {
	tx := &fakeTxManager{}
	if _, err := services.NewPatientService(nil, &fakeUserPatientRepo{}, &fakeUserPatientHistoryRepo{}, tx, discardLogger); !errors.Is(err, apperrors.ErrNilPatientRepository) {
		t.Fatalf("nil patients error = %v", err)
	}
	if _, err := services.NewPatientService(&fakePatientRepo{}, nil, &fakeUserPatientHistoryRepo{}, tx, discardLogger); !errors.Is(err, apperrors.ErrNilUserPatientRepository) {
		t.Fatalf("nil links error = %v", err)
	}
	if _, err := services.NewPatientService(&fakePatientRepo{}, &fakeUserPatientRepo{}, nil, tx, discardLogger); !errors.Is(err, apperrors.ErrNilUserPatientHistoryRepository) {
		t.Fatalf("nil history error = %v", err)
	}
	if _, err := services.NewPatientService(&fakePatientRepo{}, &fakeUserPatientRepo{}, &fakeUserPatientHistoryRepo{}, nil, discardLogger); !errors.Is(err, apperrors.ErrNilTransactionManager) {
		t.Fatalf("nil tx error = %v", err)
	}
}

func TestPatientServiceCreate(t *testing.T) {
	futurePatient := validTestPatient()
	futurePatient.DateOfBirth = time.Now().UTC().Add(time.Hour)

	invalidGender := validTestPatient()
	invalidGender.Gender = "invalid"

	invalidBlood := validTestPatient()
	invalidBlood.BloodType = "X"

	tests := []struct {
		name         string
		actingUserID uint64
		patient      *models.Patient
		relationship models.Relationship
		patientErr   error
		linkErr      error
		wantErr      error
	}{
		{name: "success", actingUserID: 7, patient: validTestPatient(), relationship: models.RelationshipSelf},
		{name: "nil patient", actingUserID: 7, patient: nil, wantErr: apperrors.ErrNilPatientRequest},
		{name: "zero acting user", actingUserID: 0, patient: validTestPatient(), relationship: models.RelationshipSelf, wantErr: apperrors.ErrInvalidPatientInput},
		{name: "empty name", actingUserID: 7, patient: func() *models.Patient { p := validTestPatient(); p.Name = "  "; return p }(), relationship: models.RelationshipSelf, wantErr: apperrors.ErrInvalidPatientInput},
		{name: "zero dob", actingUserID: 7, patient: func() *models.Patient { p := validTestPatient(); p.DateOfBirth = time.Time{}; return p }(), relationship: models.RelationshipSelf, wantErr: apperrors.ErrInvalidPatientInput},
		{name: "future dob", actingUserID: 7, patient: futurePatient, relationship: models.RelationshipSelf, wantErr: apperrors.ErrInvalidPatientInput},
		{name: "invalid gender", actingUserID: 7, patient: invalidGender, relationship: models.RelationshipSelf, wantErr: apperrors.ErrInvalidPatientInput},
		{name: "invalid blood type", actingUserID: 7, patient: invalidBlood, relationship: models.RelationshipSelf, wantErr: apperrors.ErrInvalidPatientInput},
		{name: "invalid relationship", actingUserID: 7, patient: validTestPatient(), relationship: "cousin-twice", wantErr: apperrors.ErrInvalidPatientInput},
		{name: "patient repo failure", actingUserID: 7, patient: validTestPatient(), relationship: models.RelationshipSelf, patientErr: errors.New("db down")},
		{name: "duplicate link", actingUserID: 7, patient: validTestPatient(), relationship: models.RelationshipSelf, linkErr: apperrors.ErrUserPatientAlreadyExists, wantErr: apperrors.ErrUserPatientAlreadyExists},
		{name: "link repo failure", actingUserID: 7, patient: validTestPatient(), relationship: models.RelationshipSelf, linkErr: errors.New("link down")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patients := &fakePatientRepo{
				createResult: &models.Patient{ID: 10, Name: "Asha Sharma"},
				createErr:    tt.patientErr,
			}
			links := &fakeUserPatientRepo{
				createResult: &models.UserPatient{},
				createErr:    tt.linkErr,
			}
			tx := &fakeTxManager{}
			history := &fakeUserPatientHistoryRepo{}
			service, err := services.NewPatientService(patients, links, history, tx, discardLogger)
			if err != nil {
				t.Fatalf("NewPatientService error = %v", err)
			}

			got, err := service.Create(context.Background(), tt.actingUserID, tt.patient, tt.relationship)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Create error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if tt.patientErr != nil || tt.linkErr != nil {
				if err == nil {
					t.Fatalf("Create expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Create error = %v", err)
			}
			if got == nil || got.ID != 10 {
				t.Fatalf("Create result = %+v", got)
			}
			if !tx.called {
				t.Fatalf("Create did not run inside transaction")
			}
			if !patients.createCalled || !links.createCalled {
				t.Fatalf("Create did not call both repositories")
			}
			if links.createInput.AccessLevel != models.AccessLevelFull || links.createInput.Status != models.UserPatientStatusActive {
				t.Fatalf("Create link defaults = %+v", links.createInput)
			}
			if links.createInput.UserID != tt.actingUserID || links.createInput.PatientID != 10 {
				t.Fatalf("Create link ids = %+v", links.createInput)
			}
			if len(history.entries) != 1 || history.entries[0].Reason != models.UserPatientChangeCreated || history.entries[0].PatientID != 10 {
				t.Fatalf("Create history = %+v, want one created entry for patient 10", history.entries)
			}
		})
	}
}

func TestPatientServiceCreateLinkFailureLeavesNoOrphan(t *testing.T) {
	patients := &fakePatientRepo{createResult: &models.Patient{ID: 99}}
	links := &fakeUserPatientRepo{createErr: errors.New("link down")}
	tx := &fakeTxManager{}
	service, err := services.NewPatientService(patients, links, &fakeUserPatientHistoryRepo{}, tx, discardLogger)
	if err != nil {
		t.Fatalf("NewPatientService error = %v", err)
	}

	got, err := service.Create(context.Background(), 7, validTestPatient(), models.RelationshipMother)
	if err == nil {
		t.Fatalf("Create expected link error")
	}
	if got != nil {
		t.Fatalf("Create on link failure must return nil patient, got %+v", got)
	}
	if !tx.called {
		t.Fatalf("Create must use transaction so patient+link succeed/fail together")
	}
}

func TestPatientServiceCreateCanceledContext(t *testing.T) {
	service, _ := services.NewPatientService(&fakePatientRepo{}, &fakeUserPatientRepo{}, &fakeUserPatientHistoryRepo{}, &fakeTxManager{}, discardLogger)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.Create(ctx, 7, validTestPatient(), models.RelationshipSelf); err == nil {
		t.Fatalf("Create with canceled context expected error")
	}
}

func TestPatientServiceGetByID(t *testing.T) {
	activeLink := &models.UserPatient{UserID: 7, PatientID: 10, AccessLevel: models.AccessLevelFull, Status: models.UserPatientStatusActive}

	t.Run("success", func(t *testing.T) {
		service, _ := services.NewPatientService(
			&fakePatientRepo{getResult: &models.Patient{ID: 10}},
			&fakeUserPatientRepo{activeResult: activeLink},
			&fakeUserPatientHistoryRepo{},
			&fakeTxManager{},
			discardLogger,
		)
		got, err := service.GetByID(context.Background(), 7, 10)
		if err != nil || got.Patient.ID != 10 || got.Link.AccessLevel != models.AccessLevelFull {
			t.Fatalf("GetByID = %+v, %v", got, err)
		}
	})

	t.Run("missing ids", func(t *testing.T) {
		service, _ := services.NewPatientService(&fakePatientRepo{}, &fakeUserPatientRepo{}, &fakeUserPatientHistoryRepo{}, &fakeTxManager{}, discardLogger)
		if _, err := service.GetByID(context.Background(), 0, 10); !errors.Is(err, apperrors.ErrInvalidPatientInput) {
			t.Fatalf("zero acting user error = %v", err)
		}
		if _, err := service.GetByID(context.Background(), 7, 0); !errors.Is(err, apperrors.ErrInvalidPatientInput) {
			t.Fatalf("zero patient id error = %v", err)
		}
	})

	t.Run("unrelated user does not leak existence", func(t *testing.T) {
		service, _ := services.NewPatientService(
			&fakePatientRepo{getResult: &models.Patient{ID: 50}},
			&fakeUserPatientRepo{activeErr: apperrors.ErrUserPatientNotFound},
			&fakeUserPatientHistoryRepo{},
			&fakeTxManager{},
			discardLogger,
		)
		if _, err := service.GetByID(context.Background(), 1, 50); !errors.Is(err, apperrors.ErrPatientNotFound) {
			t.Fatalf("unrelated read error = %v, want ErrPatientNotFound", err)
		}
	})

	t.Run("revoked link maps to not found", func(t *testing.T) {
		service, _ := services.NewPatientService(
			&fakePatientRepo{},
			&fakeUserPatientRepo{activeErr: apperrors.ErrUserPatientNotFound},
			&fakeUserPatientHistoryRepo{},
			&fakeTxManager{},
			discardLogger,
		)
		if _, err := service.GetByID(context.Background(), 7, 10); !errors.Is(err, apperrors.ErrPatientNotFound) {
			t.Fatalf("revoked link error = %v", err)
		}
	})

	t.Run("link dependency failure", func(t *testing.T) {
		service, _ := services.NewPatientService(
			&fakePatientRepo{},
			&fakeUserPatientRepo{activeErr: errors.New("db down")},
			&fakeUserPatientHistoryRepo{},
			&fakeTxManager{},
			discardLogger,
		)
		if _, err := service.GetByID(context.Background(), 7, 10); err == nil {
			t.Fatalf("expected link failure")
		}
	})

	t.Run("patient not found preserved", func(t *testing.T) {
		service, _ := services.NewPatientService(
			&fakePatientRepo{getErr: apperrors.ErrPatientNotFound},
			&fakeUserPatientRepo{activeResult: activeLink},
			&fakeUserPatientHistoryRepo{},
			&fakeTxManager{},
			discardLogger,
		)
		if _, err := service.GetByID(context.Background(), 7, 10); !errors.Is(err, apperrors.ErrPatientNotFound) {
			t.Fatalf("patient not found error = %v", err)
		}
	})

	t.Run("patient dependency failure", func(t *testing.T) {
		service, _ := services.NewPatientService(
			&fakePatientRepo{getErr: errors.New("db down")},
			&fakeUserPatientRepo{activeResult: activeLink},
			&fakeUserPatientHistoryRepo{},
			&fakeTxManager{},
			discardLogger,
		)
		if _, err := service.GetByID(context.Background(), 7, 10); err == nil {
			t.Fatalf("expected patient failure")
		}
	})

	t.Run("canceled context", func(t *testing.T) {
		service, _ := services.NewPatientService(&fakePatientRepo{}, &fakeUserPatientRepo{}, &fakeUserPatientHistoryRepo{}, &fakeTxManager{}, discardLogger)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := service.GetByID(ctx, 7, 10); err == nil {
			t.Fatalf("expected cancel error")
		}
	})
}

func TestPatientServiceList(t *testing.T) {
	t.Run("success preserves link order and skips missing", func(t *testing.T) {
		links := &fakeUserPatientRepo{
			listResult: []*models.UserPatient{
				{UserID: 7, PatientID: 10, Status: models.UserPatientStatusActive},
				{UserID: 7, PatientID: 11, Status: models.UserPatientStatusActive},
				{UserID: 7, PatientID: 12, Status: models.UserPatientStatusActive},
			},
		}
		patients := &fakePatientRepo{
			getByIDsResult: map[uint64]*models.Patient{
				10: {ID: 10, Name: "Self"},
				11: {ID: 11, Name: "Mother"},
			},
		}
		service, _ := services.NewPatientService(patients, links, &fakeUserPatientHistoryRepo{}, &fakeTxManager{}, discardLogger)
		got, err := service.List(context.Background(), 7, 20, 0)
		if err != nil {
			t.Fatalf("List error = %v", err)
		}
		if len(got) != 2 || got[0].Patient.ID != 10 || got[1].Patient.ID != 11 || got[1].Link.PatientID != 11 {
			t.Fatalf("List order = %+v", got)
		}
	})

	t.Run("empty links returns empty non-nil", func(t *testing.T) {
		service, _ := services.NewPatientService(
			&fakePatientRepo{},
			&fakeUserPatientRepo{listResult: nil},
			&fakeUserPatientHistoryRepo{},
			&fakeTxManager{},
			discardLogger,
		)
		got, err := service.List(context.Background(), 7, 20, 0)
		if err != nil || got == nil || len(got) != 0 {
			t.Fatalf("List empty = %+v, %v", got, err)
		}
	})

	t.Run("zero acting user", func(t *testing.T) {
		service, _ := services.NewPatientService(&fakePatientRepo{}, &fakeUserPatientRepo{}, &fakeUserPatientHistoryRepo{}, &fakeTxManager{}, discardLogger)
		if _, err := service.List(context.Background(), 0, 20, 0); !errors.Is(err, apperrors.ErrInvalidPatientInput) {
			t.Fatalf("zero user error = %v", err)
		}
	})

	t.Run("dependency failures", func(t *testing.T) {
		badLinks, _ := services.NewPatientService(&fakePatientRepo{}, &fakeUserPatientRepo{listErr: errors.New("db down")}, &fakeUserPatientHistoryRepo{}, &fakeTxManager{}, discardLogger)
		if _, err := badLinks.List(context.Background(), 7, 20, 0); err == nil {
			t.Fatalf("expected list links failure")
		}
		badPatients, _ := services.NewPatientService(
			&fakePatientRepo{getByIDsErr: errors.New("db down")},
			&fakeUserPatientRepo{listResult: []*models.UserPatient{{UserID: 7, PatientID: 10, Status: models.UserPatientStatusActive}}},
			&fakeUserPatientHistoryRepo{},
			&fakeTxManager{},
			discardLogger,
		)
		if _, err := badPatients.List(context.Background(), 7, 20, 0); err == nil {
			t.Fatalf("expected GetByIDs failure")
		}
	})

	t.Run("pagination clamped", func(t *testing.T) {
		links := &fakeUserPatientRepo{listResult: nil}
		service, _ := services.NewPatientService(&fakePatientRepo{}, links, &fakeUserPatientHistoryRepo{}, &fakeTxManager{}, discardLogger)
		if _, err := service.List(context.Background(), 7, 5000, -5); err != nil {
			t.Fatalf("List clamp error = %v", err)
		}
		if links.listLimit != 100 || links.listOffset != 0 {
			t.Fatalf("clamped pagination = %d/%d", links.listLimit, links.listOffset)
		}
		if _, err := service.List(context.Background(), 7, 0, 0); err != nil {
			t.Fatalf("List default error = %v", err)
		}
		if links.listLimit != 20 {
			t.Fatalf("default limit = %d", links.listLimit)
		}
	})

	t.Run("canceled context", func(t *testing.T) {
		service, _ := services.NewPatientService(&fakePatientRepo{}, &fakeUserPatientRepo{}, &fakeUserPatientHistoryRepo{}, &fakeTxManager{}, discardLogger)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := service.List(ctx, 7, 20, 0); err == nil {
			t.Fatalf("expected cancel error")
		}
	})
}

func TestPatientServiceUpdate(t *testing.T) {
	fullLink := &models.UserPatient{UserID: 7, PatientID: 10, AccessLevel: models.AccessLevelFull, Status: models.UserPatientStatusActive}
	readOnly := &models.UserPatient{UserID: 7, PatientID: 10, AccessLevel: models.AccessLevelReadOnly, Status: models.UserPatientStatusActive}
	apptOnly := &models.UserPatient{UserID: 7, PatientID: 10, AccessLevel: models.AccessLevelAppointmentsOnly, Status: models.UserPatientStatusActive}

	t.Run("success updates demographics only", func(t *testing.T) {
		patients := &fakePatientRepo{updateResult: &models.Patient{ID: 10, Name: "Updated"}}
		links := &fakeUserPatientRepo{activeResult: fullLink}
		service, _ := services.NewPatientService(patients, links, &fakeUserPatientHistoryRepo{}, &fakeTxManager{}, discardLogger)

		input := validTestPatient()
		input.ID = 10
		input.Name = "Updated"
		got, err := service.Update(context.Background(), 7, input)
		if err != nil || got.Name != "Updated" {
			t.Fatalf("Update = %+v, %v", got, err)
		}
		if patients.updateInput.ID != 10 || patients.updateInput.Name != "Updated" {
			t.Fatalf("Update input = %+v", patients.updateInput)
		}
		if links.updateCalled {
			t.Fatalf("Update must not mutate user-patient link")
		}
	})

	t.Run("validation", func(t *testing.T) {
		service, _ := services.NewPatientService(&fakePatientRepo{}, &fakeUserPatientRepo{activeResult: fullLink}, &fakeUserPatientHistoryRepo{}, &fakeTxManager{}, discardLogger)
		if _, err := service.Update(context.Background(), 7, nil); !errors.Is(err, apperrors.ErrNilPatientRequest) {
			t.Fatalf("nil error = %v", err)
		}
		noID := validTestPatient()
		if _, err := service.Update(context.Background(), 7, noID); !errors.Is(err, apperrors.ErrInvalidPatientInput) {
			t.Fatalf("zero id error = %v", err)
		}
		if _, err := service.Update(context.Background(), 0, func() *models.Patient { p := validTestPatient(); p.ID = 10; return p }()); !errors.Is(err, apperrors.ErrInvalidPatientInput) {
			t.Fatalf("zero user error = %v", err)
		}
		bad := validTestPatient()
		bad.ID = 10
		bad.Name = ""
		if _, err := service.Update(context.Background(), 7, bad); !errors.Is(err, apperrors.ErrInvalidPatientInput) {
			t.Fatalf("bad name error = %v", err)
		}
	})

	t.Run("missing link maps to not found", func(t *testing.T) {
		service, _ := services.NewPatientService(
			&fakePatientRepo{},
			&fakeUserPatientRepo{activeErr: apperrors.ErrUserPatientNotFound},
			&fakeUserPatientHistoryRepo{},
			&fakeTxManager{},
			discardLogger,
		)
		input := validTestPatient()
		input.ID = 10
		if _, err := service.Update(context.Background(), 7, input); !errors.Is(err, apperrors.ErrPatientNotFound) {
			t.Fatalf("missing link error = %v", err)
		}
	})

	t.Run("insufficient access forbidden", func(t *testing.T) {
		for _, link := range []*models.UserPatient{readOnly, apptOnly} {
			service, _ := services.NewPatientService(
				&fakePatientRepo{},
				&fakeUserPatientRepo{activeResult: link},
				&fakeUserPatientHistoryRepo{},
				&fakeTxManager{},
				discardLogger,
			)
			input := validTestPatient()
			input.ID = 10
			if _, err := service.Update(context.Background(), 7, input); !errors.Is(err, apperrors.ErrPatientForbidden) {
				t.Fatalf("access %q error = %v", link.AccessLevel, err)
			}
		}
	})

	t.Run("patient repo failures", func(t *testing.T) {
		notFound, _ := services.NewPatientService(
			&fakePatientRepo{updateErr: apperrors.ErrPatientNotFound},
			&fakeUserPatientRepo{activeResult: fullLink},
			&fakeUserPatientHistoryRepo{},
			&fakeTxManager{},
			discardLogger,
		)
		input := validTestPatient()
		input.ID = 10
		if _, err := notFound.Update(context.Background(), 7, input); !errors.Is(err, apperrors.ErrPatientNotFound) {
			t.Fatalf("not found error = %v", err)
		}
		broken, _ := services.NewPatientService(
			&fakePatientRepo{updateErr: errors.New("db down")},
			&fakeUserPatientRepo{activeResult: fullLink},
			&fakeUserPatientHistoryRepo{},
			&fakeTxManager{},
			discardLogger,
		)
		if _, err := broken.Update(context.Background(), 7, input); err == nil {
			t.Fatalf("expected update failure")
		}
	})

	t.Run("canceled context", func(t *testing.T) {
		service, _ := services.NewPatientService(&fakePatientRepo{}, &fakeUserPatientRepo{}, &fakeUserPatientHistoryRepo{}, &fakeTxManager{}, discardLogger)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		input := validTestPatient()
		input.ID = 10
		if _, err := service.Update(ctx, 7, input); err == nil {
			t.Fatalf("expected cancel error")
		}
	})
}
