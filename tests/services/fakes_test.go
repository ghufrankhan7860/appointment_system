package services_test

import (
	"context"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
)

type fakeAdminUserRepo struct {
	adminExists bool
	createInput *models.User
	createErr   error
}

func (f *fakeAdminUserRepo) Create(ctx context.Context, user *models.User) (*models.User, error) {
	f.createInput = user
	if f.createErr != nil {
		return nil, f.createErr
	}
	created := *user
	created.ID = 1
	return &created, nil
}

func (f *fakeAdminUserRepo) FindByLogin(ctx context.Context, identifier string) (*models.User, error) {
	return nil, apperrors.ErrInvalidCredentials
}

func (f *fakeAdminUserRepo) GetByID(ctx context.Context, id uint64) (*models.User, error) {
	return nil, apperrors.ErrUserNotFound
}

func (f *fakeAdminUserRepo) ExistsByRole(ctx context.Context, role models.Role) (bool, error) {
	return f.adminExists, nil
}

type fakeUserRepo struct {
	user *models.User
	err  error
}

func (f *fakeUserRepo) Create(ctx context.Context, user *models.User) (*models.User, error) {
	return f.user, f.err
}

func (f *fakeUserRepo) FindByLogin(ctx context.Context, identifier string) (*models.User, error) {
	return f.user, f.err
}

func (f *fakeUserRepo) ExistsByRole(ctx context.Context, role models.Role) (bool, error) {
	return f.user != nil, f.err
}

func (f *fakeUserRepo) GetByID(ctx context.Context, id uint64) (*models.User, error) {
	return f.user, f.err
}

type fakeSessionRepo struct {
	session      *models.Session
	getErr       error
	createErr    error
	revokeErr    error
	createCalled bool
	revokeCalled bool
}

func (f *fakeSessionRepo) Create(ctx context.Context, session *models.Session) error {
	f.createCalled = true
	return f.createErr
}

func (f *fakeSessionRepo) GetByTokenHash(ctx context.Context, tokenHash string) (*models.Session, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.session, nil
}

func (f *fakeSessionRepo) RevokeByID(ctx context.Context, id uint64) error {
	f.revokeCalled = true
	return f.revokeErr
}

type fakeTokenGen struct {
	token string
	err   error
}

func (f *fakeTokenGen) Generate(user models.User) (string, error) {
	return f.token, f.err
}

type fakeAvailabilityRepo struct {
	rules          []*models.AvailabilityRule
	overrides      []*models.AvailabilityOverride
	replaceInput   []models.AvailabilityRule
	replaceCalled  bool
	createdInput   *models.AvailabilityOverride
	listRulesDay   time.Weekday
	listOverrideTo time.Time
	err            error
}

func (f *fakeAvailabilityRepo) ReplaceRules(ctx context.Context, doctorID uint64, rules []models.AvailabilityRule) ([]*models.AvailabilityRule, error) {
	f.replaceCalled = true
	f.replaceInput = rules
	if f.err != nil {
		return nil, f.err
	}
	result := make([]*models.AvailabilityRule, 0, len(rules))
	for index := range rules {
		rule := rules[index]
		result = append(result, &rule)
	}
	return result, nil
}

func (f *fakeAvailabilityRepo) ListRules(ctx context.Context, doctorID uint64) ([]*models.AvailabilityRule, error) {
	return f.rules, f.err
}

func (f *fakeAvailabilityRepo) ListRulesByDay(ctx context.Context, doctorID uint64, dayOfWeek time.Weekday) ([]*models.AvailabilityRule, error) {
	f.listRulesDay = dayOfWeek
	result := []*models.AvailabilityRule{}
	for _, rule := range f.rules {
		if rule.DayOfWeek == dayOfWeek {
			result = append(result, rule)
		}
	}
	return result, f.err
}

func (f *fakeAvailabilityRepo) CreateOverride(ctx context.Context, override *models.AvailabilityOverride) (*models.AvailabilityOverride, error) {
	f.createdInput = override
	if f.err != nil {
		return nil, f.err
	}
	created := *override
	created.ID = 9
	return &created, nil
}

func (f *fakeAvailabilityRepo) ListOverrides(ctx context.Context, doctorID uint64, fromDate time.Time, toDate time.Time) ([]*models.AvailabilityOverride, error) {
	f.listOverrideTo = toDate
	return f.overrides, f.err
}

type fakeDoctorRepo struct {
	byUserID     *models.Doctor
	getByUserErr error
	byID         *models.Doctor
	getByIDErr   error

	createErr   error
	createInput *models.Doctor
	updateErr   error
	updateInput *models.Doctor

	updateStatusErr    error
	updateStatusCalled bool
	updateStatusFrom   models.VerificationStatus
	updateStatusTo     models.VerificationStatus

	searchInput  models.DoctorSearch
	searchResult []*models.Doctor
	listStatus   models.VerificationStatus
	listLimit    int
}

func (f *fakeDoctorRepo) Create(ctx context.Context, doctor *models.Doctor) (*models.Doctor, error) {
	f.createInput = doctor
	if f.createErr != nil {
		return nil, f.createErr
	}
	created := *doctor
	created.ID = 100
	return &created, nil
}

func (f *fakeDoctorRepo) GetByID(ctx context.Context, id uint64) (*models.Doctor, error) {
	if f.getByIDErr != nil {
		return nil, f.getByIDErr
	}
	if f.byID == nil {
		return nil, apperrors.ErrDoctorNotFound
	}
	found := *f.byID
	return &found, nil
}

func (f *fakeDoctorRepo) GetByUserID(ctx context.Context, userID uint64) (*models.Doctor, error) {
	if f.getByUserErr != nil {
		return nil, f.getByUserErr
	}
	if f.byUserID == nil {
		return nil, apperrors.ErrDoctorNotFound
	}
	found := *f.byUserID
	return &found, nil
}

func (f *fakeDoctorRepo) Update(ctx context.Context, doctor *models.Doctor) (*models.Doctor, error) {
	f.updateInput = doctor
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	updated := *doctor
	return &updated, nil
}

func (f *fakeDoctorRepo) UpdateStatus(ctx context.Context, id uint64, fromStatus models.VerificationStatus, toStatus models.VerificationStatus, verifiedAt *time.Time) error {
	f.updateStatusCalled = true
	f.updateStatusFrom = fromStatus
	f.updateStatusTo = toStatus
	return f.updateStatusErr
}

func (f *fakeDoctorRepo) ListByStatus(ctx context.Context, status models.VerificationStatus, limit int, offset int) ([]*models.Doctor, error) {
	f.listStatus = status
	f.listLimit = limit
	return []*models.Doctor{}, nil
}

func (f *fakeDoctorRepo) Search(ctx context.Context, search models.DoctorSearch) ([]*models.Doctor, error) {
	f.searchInput = search
	return f.searchResult, nil
}

type fakeSpecialityRepo struct {
	known map[string]*models.Speciality
	err   error
}

func (f *fakeSpecialityRepo) Create(ctx context.Context, speciality *models.Speciality) (*models.Speciality, error) {
	if f.err != nil {
		return nil, f.err
	}
	created := *speciality
	created.ID = 50
	return &created, nil
}

func (f *fakeSpecialityRepo) GetBySlugs(ctx context.Context, slugs []string) (map[string]*models.Speciality, error) {
	if f.err != nil {
		return nil, f.err
	}
	result := map[string]*models.Speciality{}
	for _, slug := range slugs {
		if speciality, ok := f.known[slug]; ok {
			result[slug] = speciality
		}
	}
	return result, nil
}

func (f *fakeSpecialityRepo) List(ctx context.Context) ([]*models.Speciality, error) {
	return nil, f.err
}

type fakeDoctorVerificationRepo struct {
	created []*models.DoctorVerification
	err     error
}

func (f *fakeDoctorVerificationRepo) Create(ctx context.Context, verification *models.DoctorVerification) error {
	if f.err != nil {
		return f.err
	}
	f.created = append(f.created, verification)
	return nil
}

func (f *fakeDoctorVerificationRepo) ListByDoctor(ctx context.Context, doctorID uint64) ([]*models.DoctorVerification, error) {
	return f.created, f.err
}

type fakePatientRepo struct {
	createResult *models.Patient
	createErr    error
	createCalled bool
	createCtx    context.Context

	getResult *models.Patient
	getErr    error

	getByIDsResult map[uint64]*models.Patient
	getByIDsErr    error
	getByIDsLimit  []uint64

	updateResult *models.Patient
	updateErr    error
	updateInput  *models.Patient
	updateCalled bool
}

func (f *fakePatientRepo) Create(ctx context.Context, patient *models.Patient) (*models.Patient, error) {
	f.createCalled = true
	f.createCtx = ctx
	if f.createErr != nil {
		return nil, f.createErr
	}
	return f.createResult, nil
}

func (f *fakePatientRepo) GetByID(ctx context.Context, id uint64) (*models.Patient, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.getResult, nil
}

func (f *fakePatientRepo) GetByIDs(ctx context.Context, ids []uint64) (map[uint64]*models.Patient, error) {
	f.getByIDsLimit = ids
	if f.getByIDsErr != nil {
		return nil, f.getByIDsErr
	}
	return f.getByIDsResult, nil
}

func (f *fakePatientRepo) Update(ctx context.Context, patient *models.Patient) (*models.Patient, error) {
	f.updateCalled = true
	f.updateInput = patient
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return f.updateResult, nil
}

type fakeUserPatientRepo struct {
	createResult *models.UserPatient
	createErr    error
	createInput  *models.UserPatient
	createCalled bool

	activeResult *models.UserPatient
	activeErr    error

	listResult []*models.UserPatient
	listErr    error
	listLimit  int
	listOffset int

	updateCalled bool
}

func (f *fakeUserPatientRepo) Create(ctx context.Context, link *models.UserPatient) (*models.UserPatient, error) {
	f.createCalled = true
	f.createInput = link
	if f.createErr != nil {
		return nil, f.createErr
	}
	return f.createResult, nil
}

func (f *fakeUserPatientRepo) GetByUserAndPatient(ctx context.Context, userID uint64, patientID uint64) (*models.UserPatient, error) {
	return f.activeResult, f.activeErr
}

func (f *fakeUserPatientRepo) GetActiveByUserAndPatient(ctx context.Context, userID uint64, patientID uint64) (*models.UserPatient, error) {
	if f.activeErr != nil {
		return nil, f.activeErr
	}
	return f.activeResult, nil
}

func (f *fakeUserPatientRepo) ListByPatient(ctx context.Context, patientID uint64, limit int, offset int) ([]*models.UserPatient, error) {
	return nil, nil
}

func (f *fakeUserPatientRepo) ListByUser(ctx context.Context, userID uint64, limit int, offset int) ([]*models.UserPatient, error) {
	return f.ListActiveByUser(ctx, userID, limit, offset)
}

func (f *fakeUserPatientRepo) ListActiveByUser(ctx context.Context, userID uint64, limit int, offset int) ([]*models.UserPatient, error) {
	f.listLimit = limit
	f.listOffset = offset
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.listResult, nil
}

func (f *fakeUserPatientRepo) Update(ctx context.Context, link *models.UserPatient) (*models.UserPatient, error) {
	f.updateCalled = true
	return f.activeResult, f.activeErr
}

func (f *fakeUserPatientRepo) Revoke(ctx context.Context, userID uint64, patientID uint64) (*models.UserPatient, error) {
	return f.activeResult, f.activeErr
}

type fakeTxManager struct {
	called  bool
	txErr   error
	innerCT context.Context
}

func (f *fakeTxManager) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	f.called = true
	if f.txErr != nil {
		return f.txErr
	}
	return fn(ctx)
}
