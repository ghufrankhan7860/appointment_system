package handler_test

import (
	"context"
	"errors"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
)


type fakePatientRepo struct {
	patient *models.Patient
	err     error
}

func (f *fakePatientRepo) Create(ctx context.Context, patient *models.Patient) (*models.Patient, error) {
	return f.patient, f.err
}

func (f *fakePatientRepo) GetByID(ctx context.Context, id uint64) (*models.Patient, error) {
	return f.patient, f.err
}

func (f *fakePatientRepo) GetByIDs(ctx context.Context, ids []uint64) (map[uint64]*models.Patient, error) {
	if f.err != nil {
		return nil, f.err
	}
	return map[uint64]*models.Patient{f.patient.ID: f.patient}, nil
}

func (f *fakePatientRepo) Update(ctx context.Context, patient *models.Patient) (*models.Patient, error) {
	return f.patient, f.err
}

type fakeUserPatientRepo struct {
	userPatient *models.UserPatient
	createErr   error
	getErr      error
}

func (f *fakeUserPatientRepo) Create(ctx context.Context, userPatient *models.UserPatient) (*models.UserPatient, error) {
	return userPatient, f.createErr
}

func (f *fakeUserPatientRepo) GetByUserAndPatient(ctx context.Context, userID uint64, patientID uint64) (*models.UserPatient, error) {
	return f.userPatient, f.getErr
}

func (f *fakeUserPatientRepo) GetActiveByUserAndPatient(ctx context.Context, userID uint64, patientID uint64) (*models.UserPatient, error) {
	return f.userPatient, f.getErr
}

func (f *fakeUserPatientRepo) ListByPatient(ctx context.Context, patientID uint64, limit int, offset int) ([]*models.UserPatient, error) {
	return []*models.UserPatient{f.userPatient}, f.getErr
}

func (f *fakeUserPatientRepo) ListByUser(ctx context.Context, userID uint64, limit int, offset int) ([]*models.UserPatient, error) {
	return []*models.UserPatient{f.userPatient}, f.getErr
}

func (f *fakeUserPatientRepo) ListActiveByUser(ctx context.Context, userID uint64, limit int, offset int) ([]*models.UserPatient, error) {
	return []*models.UserPatient{f.userPatient}, f.getErr
}

func (f *fakeUserPatientRepo) Update(ctx context.Context, userPatient *models.UserPatient) (*models.UserPatient, error) {
	return userPatient, nil
}

func (f *fakeUserPatientRepo) Revoke(ctx context.Context, userID uint64, patientID uint64) (*models.UserPatient, error) {
	return f.userPatient, nil
}

type fakeTxManager struct{}

func (f *fakeTxManager) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type fakeTokenVerifier struct{}

func (f *fakeTokenVerifier) Verify(token string) (uint64, string, error) {
	if token != "good-token" {
		return 0, "", errors.New("invalid token")
	}
	return 7, "patient", nil
}

type fakeRoleTokenVerifier struct{}

func (f *fakeRoleTokenVerifier) Verify(token string) (uint64, string, error) {
	switch token {
	case "admin-token":
		return 1, "admin", nil
	case "doctor-token":
		return 11, "doctor", nil
	case "patient-token":
		return 7, "patient", nil
	default:
		return 0, "", errors.New("invalid token")
	}
}

type fakeDoctorRepo struct {
	doctor    *models.Doctor
	createErr error
}

func (f *fakeDoctorRepo) Create(ctx context.Context, doctor *models.Doctor) (*models.Doctor, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	created := *doctor
	created.ID = 5
	f.doctor = &created
	return &created, nil
}

func (f *fakeDoctorRepo) GetByID(ctx context.Context, id uint64) (*models.Doctor, error) {
	if f.doctor == nil || f.doctor.ID != id {
		return nil, apperrors.ErrDoctorNotFound
	}
	found := *f.doctor
	return &found, nil
}

func (f *fakeDoctorRepo) GetByUserID(ctx context.Context, userID uint64) (*models.Doctor, error) {
	if f.doctor == nil || f.doctor.UserID != userID {
		return nil, apperrors.ErrDoctorNotFound
	}
	found := *f.doctor
	return &found, nil
}

func (f *fakeDoctorRepo) Update(ctx context.Context, doctor *models.Doctor) (*models.Doctor, error) {
	updated := *doctor
	f.doctor = &updated
	return &updated, nil
}

func (f *fakeDoctorRepo) UpdateStatus(ctx context.Context, id uint64, fromStatus models.VerificationStatus, toStatus models.VerificationStatus, verifiedAt *time.Time) error {
	if f.doctor == nil || f.doctor.ID != id || f.doctor.VerificationStatus != fromStatus {
		return apperrors.ErrInvalidVerificationTransition
	}
	f.doctor.VerificationStatus = toStatus
	f.doctor.VerifiedAt = verifiedAt
	return nil
}

func (f *fakeDoctorRepo) ListByStatus(ctx context.Context, status models.VerificationStatus, limit int, offset int) ([]*models.Doctor, error) {
	if f.doctor == nil || (status != "" && f.doctor.VerificationStatus != status) {
		return []*models.Doctor{}, nil
	}
	return []*models.Doctor{f.doctor}, nil
}

func (f *fakeDoctorRepo) Search(ctx context.Context, search models.DoctorSearch) ([]*models.Doctor, error) {
	if f.doctor == nil || f.doctor.VerificationStatus != models.VerificationStatusVerified {
		return []*models.Doctor{}, nil
	}
	return []*models.Doctor{f.doctor}, nil
}

type fakeSpecialityRepo struct{}

func (f *fakeSpecialityRepo) Create(ctx context.Context, speciality *models.Speciality) (*models.Speciality, error) {
	created := *speciality
	created.ID = 50
	return &created, nil
}

func (f *fakeSpecialityRepo) GetBySlugs(ctx context.Context, slugs []string) (map[string]*models.Speciality, error) {
	result := map[string]*models.Speciality{}
	for _, slug := range slugs {
		if slug == "general-medicine" {
			result[slug] = &models.Speciality{ID: 1, Slug: slug, Name: "General Medicine"}
		}
	}
	return result, nil
}

func (f *fakeSpecialityRepo) List(ctx context.Context) ([]*models.Speciality, error) {
	return []*models.Speciality{{ID: 1, Slug: "general-medicine", Name: "General Medicine"}}, nil
}

type fakeDoctorVerificationRepo struct{}

func (f *fakeDoctorVerificationRepo) Create(ctx context.Context, verification *models.DoctorVerification) error {
	return nil
}

func (f *fakeDoctorVerificationRepo) ListByDoctor(ctx context.Context, doctorID uint64) ([]*models.DoctorVerification, error) {
	return []*models.DoctorVerification{}, nil
}

type fakeAvailabilityRepo struct {
	rules []*models.AvailabilityRule
}

func (f *fakeAvailabilityRepo) ReplaceRules(ctx context.Context, doctorID uint64, rules []models.AvailabilityRule) ([]*models.AvailabilityRule, error) {
	f.rules = make([]*models.AvailabilityRule, 0, len(rules))
	for index := range rules {
		rule := rules[index]
		f.rules = append(f.rules, &rule)
	}
	return f.rules, nil
}

func (f *fakeAvailabilityRepo) ListRules(ctx context.Context, doctorID uint64) ([]*models.AvailabilityRule, error) {
	return f.rules, nil
}

func (f *fakeAvailabilityRepo) ListRulesByDay(ctx context.Context, doctorID uint64, dayOfWeek time.Weekday) ([]*models.AvailabilityRule, error) {
	result := []*models.AvailabilityRule{}
	for _, rule := range f.rules {
		if rule.DayOfWeek == dayOfWeek {
			result = append(result, rule)
		}
	}
	return result, nil
}

func (f *fakeAvailabilityRepo) CreateOverride(ctx context.Context, override *models.AvailabilityOverride) (*models.AvailabilityOverride, error) {
	created := *override
	created.ID = 9
	return &created, nil
}

func (f *fakeAvailabilityRepo) ListOverrides(ctx context.Context, doctorID uint64, fromDate time.Time, toDate time.Time) ([]*models.AvailabilityOverride, error) {
	return []*models.AvailabilityOverride{}, nil
}

type fakeUserRepo struct{}

func (f *fakeUserRepo) Create(ctx context.Context, user *models.User) (*models.User, error) {
	created := *user
	created.ID = 2
	return &created, nil
}

func (f *fakeUserRepo) FindByLogin(ctx context.Context, identifier string) (*models.User, error) {
	return nil, apperrors.ErrInvalidCredentials
}

func (f *fakeUserRepo) GetByID(ctx context.Context, id uint64) (*models.User, error) {
	return nil, apperrors.ErrUserNotFound
}

func (f *fakeUserRepo) ExistsByRole(ctx context.Context, role models.Role) (bool, error) {
	return true, nil
}
