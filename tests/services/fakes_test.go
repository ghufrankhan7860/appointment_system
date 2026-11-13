package services_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
)

// discardLogger satisfies the services that log; tests that check a log line
// build their own JSON logger.
var discardLogger = slog.New(slog.DiscardHandler)

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

func (f *fakeAdminUserRepo) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	return nil, apperrors.ErrInvalidCredentials
}

func (f *fakeAdminUserRepo) FindByMobile(ctx context.Context, mobileNo string) (*models.User, error) {
	return nil, apperrors.ErrInvalidCredentials
}

func (f *fakeAdminUserRepo) GetByID(ctx context.Context, id uint64) (*models.User, error) {
	return nil, apperrors.ErrUserNotFound
}

func (f *fakeAdminUserRepo) ExistsByRole(ctx context.Context, role models.Role) (bool, error) {
	return f.adminExists, nil
}

func (f *fakeAdminUserRepo) GetByIDs(ctx context.Context, ids []uint64) (map[uint64]*models.User, error) {
	return map[uint64]*models.User{}, nil
}

type fakeUserRepo struct {
	user       *models.User
	users      map[uint64]*models.User
	err        error
	lookedUpBy string
}

func (f *fakeUserRepo) GetByIDs(ctx context.Context, ids []uint64) (map[uint64]*models.User, error) {
	return f.users, f.err
}

func (f *fakeUserRepo) Create(ctx context.Context, user *models.User) (*models.User, error) {
	return f.user, f.err
}

func (f *fakeUserRepo) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	f.lookedUpBy = "email:" + email
	return f.user, f.err
}

func (f *fakeUserRepo) FindByMobile(ctx context.Context, mobileNo string) (*models.User, error) {
	f.lookedUpBy = "mobile:" + mobileNo
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
	sessions     []*models.Session
	getErr       error
	createErr    error
	revokeErr    error
	rotateErr    error
	listErr      error
	createCalled bool
	revokeCalled bool

	created       *models.Session
	rotated       *models.Session
	rotatedFrom   string
	listedNow     time.Time
	listedLimit   int
	listedOffset  int
	revokedUserID uint64
	revokedID     uint64
	exceptID      uint64
	revokedCount  int64
	deleteBefore  time.Time
	deleteLimit   int
	deleteResults []int64
	deleteCalls   int
}

func (f *fakeSessionRepo) Create(ctx context.Context, session *models.Session) error {
	f.createCalled = true
	if f.createErr != nil {
		return f.createErr
	}
	session.ID = 41
	created := *session
	f.created = &created
	return nil
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

func (f *fakeSessionRepo) Rotate(ctx context.Context, id uint64, oldTokenHash string, session *models.Session) error {
	if f.rotateErr != nil {
		return f.rotateErr
	}
	rotated := *session
	f.rotated = &rotated
	f.rotatedFrom = oldTokenHash
	return nil
}

func (f *fakeSessionRepo) ListActiveByUser(ctx context.Context, userID uint64, now time.Time) ([]*models.Session, error) {
	f.listedNow = now
	return f.sessions, f.listErr
}

func (f *fakeSessionRepo) ListByUser(ctx context.Context, userID uint64, limit int, offset int) ([]*models.Session, error) {
	f.listedLimit = limit
	f.listedOffset = offset
	return f.sessions, f.listErr
}

func (f *fakeSessionRepo) RevokeByUserAndID(ctx context.Context, userID uint64, id uint64) error {
	f.revokedUserID = userID
	f.revokedID = id
	return f.revokeErr
}

func (f *fakeSessionRepo) RevokeActiveByUser(ctx context.Context, userID uint64, exceptID uint64) (int64, error) {
	f.revokedUserID = userID
	f.exceptID = exceptID
	return f.revokedCount, f.revokeErr
}

func (f *fakeSessionRepo) DeleteEndedBefore(ctx context.Context, before time.Time, limit int) (int64, error) {
	f.deleteBefore = before
	f.deleteLimit = limit
	if f.deleteCalls >= len(f.deleteResults) {
		return 0, nil
	}
	deleted := f.deleteResults[f.deleteCalls]
	f.deleteCalls++
	return deleted, nil
}

type fakeTokenGen struct {
	token     string
	err       error
	sessionID uint64
}

func (f *fakeTokenGen) Generate(user models.User, sessionID uint64) (string, error) {
	f.sessionID = sessionID
	return f.token, f.err
}

type fakeUserAgentParser struct {
	details models.UserAgentDetails
	parsed  string
}

func (f *fakeUserAgentParser) Parse(userAgent string) models.UserAgentDetails {
	f.parsed = userAgent
	return f.details
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
		if rule.DayOfWeek == dayOfWeek && (rule.DoctorID == 0 || rule.DoctorID == doctorID) {
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
	searchCalls  int
	listStatus   models.VerificationStatus
	listLimit    int

	upiID    string
	upiQRKey *string
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
	f.searchCalls++
	if search.Limit == 0 || search.Offset >= len(f.searchResult) {
		return f.searchResult[min(search.Offset, len(f.searchResult)):], nil
	}
	return f.searchResult[search.Offset:min(search.Offset+search.Limit, len(f.searchResult))], nil
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

	patientLinks []*models.UserPatient

	updateCalled  bool
	updateInput   *models.UserPatient
	updateErr     error
	revokedUserID uint64
}

func (f *fakeUserPatientRepo) ListActiveByPatient(ctx context.Context, patientID uint64) ([]*models.UserPatient, error) {
	return f.patientLinks, nil
}

func (f *fakeUserPatientRepo) ListActiveByPatientForUpdate(ctx context.Context, patientID uint64) ([]*models.UserPatient, error) {
	return f.patientLinks, nil
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
	f.updateInput = link
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	updated := *link
	return &updated, nil
}

func (f *fakeUserPatientRepo) Revoke(ctx context.Context, userID uint64, patientID uint64) (*models.UserPatient, error) {
	f.revokedUserID = userID
	return &models.UserPatient{UserID: userID, PatientID: patientID, Status: models.UserPatientStatusRevoked}, nil
}

type fakePatientInviteRepo struct {
	invite  *models.PatientInvite
	getErr  error
	gotHash string

	createErrs  []error
	createCalls int
	created     *models.PatientInvite

	pending    int64
	listResult []*models.PatientInvite

	revokeErr  error
	revokedID  uint64
	redeemedID uint64
	expiredID  uint64
}

func (f *fakePatientInviteRepo) Create(ctx context.Context, invite *models.PatientInvite) (*models.PatientInvite, error) {
	f.createCalls++
	if len(f.createErrs) >= f.createCalls && f.createErrs[f.createCalls-1] != nil {
		return nil, f.createErrs[f.createCalls-1]
	}
	created := *invite
	created.ID = 31
	f.created = &created
	return &created, nil
}

func (f *fakePatientInviteRepo) GetByCodeHashForUpdate(ctx context.Context, codeHash string) (*models.PatientInvite, error) {
	f.gotHash = codeHash
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.invite, nil
}

func (f *fakePatientInviteRepo) ListPendingByPatient(ctx context.Context, patientID uint64, now time.Time) ([]*models.PatientInvite, error) {
	return f.listResult, nil
}

func (f *fakePatientInviteRepo) CountPendingByPatient(ctx context.Context, patientID uint64, now time.Time) (int64, error) {
	return f.pending, nil
}

func (f *fakePatientInviteRepo) MarkRedeemed(ctx context.Context, id uint64, redeemedByUserID uint64, redeemedAt time.Time) error {
	f.redeemedID = id
	return nil
}

func (f *fakePatientInviteRepo) Revoke(ctx context.Context, patientID uint64, id uint64) error {
	f.revokedID = id
	return f.revokeErr
}

func (f *fakePatientInviteRepo) MarkExpired(ctx context.Context, id uint64) error {
	f.expiredID = id
	return nil
}

type fakeInviteAttemptRepo struct {
	count         int64
	created       int
	deletedBefore time.Time
}

func (f *fakeInviteAttemptRepo) Create(ctx context.Context, userID uint64, at time.Time) error {
	f.created++
	return nil
}

func (f *fakeInviteAttemptRepo) CountSince(ctx context.Context, userID uint64, since time.Time) (int64, error) {
	return f.count, nil
}

func (f *fakeInviteAttemptRepo) DeleteBefore(ctx context.Context, userID uint64, before time.Time) error {
	f.deletedBefore = before
	return nil
}

type fakeUserPatientHistoryRepo struct {
	entries []*models.UserPatientHistory
}

func (f *fakeUserPatientHistoryRepo) Create(ctx context.Context, history *models.UserPatientHistory) error {
	f.entries = append(f.entries, history)
	return nil
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

type fakeAppointmentRepo struct {
	byID   *models.AppointmentDetails
	getErr error
	byKey  *models.AppointmentDetails
	booked []*models.AppointmentDetails

	updateStatusErr error
	updateStatusTo  models.AppointmentStatus

	createErr   error
	createInput *models.Appointment
	created     []*models.Appointment
	clashWinner *models.AppointmentDetails

	cancelErr    error
	cancelCalled bool
	cancelReason models.CancelReason
	cancelledIDs []uint64

	statusUpdates []string

	proposal         *models.RescheduleProposal
	createdProposal  *models.RescheduleProposal
	closedTo         models.ProposalStatus
	closedBy         uint64
	closeErr         error
	expiredProposals []*models.RescheduleProposal

	statusChanges []*models.AppointmentStatusChange

	listFilter models.AppointmentFilter
	listFrom   time.Time
	listTo     time.Time

	markPaidErr     error
	markPaidCalled  bool
	paymentStatusTo models.AppointmentPaymentStatus
	unpaidDue       []*models.AppointmentDetails
	cancelUnpaidErr error
	cancelUnpaidIDs []uint64
	paymentDueAt    *time.Time

	due      map[models.ReminderWindow][]*models.AppointmentDetails
	dueFrom  map[models.ReminderWindow]time.Time
	reminded map[string]bool

	patients      []*models.DoctorPatient
	patientsQuery string
	patientsLimit int
	treated       map[uint64]bool
}

func (f *fakeAppointmentRepo) ListDueForReminder(ctx context.Context, window models.ReminderWindow, from time.Time, to time.Time, limit int) ([]*models.AppointmentDetails, error) {
	if f.dueFrom == nil {
		f.dueFrom = map[models.ReminderWindow]time.Time{}
	}
	f.dueFrom[window] = from
	return f.due[window], nil
}

func (f *fakeAppointmentRepo) CreateReminder(ctx context.Context, appointmentID uint64, window models.ReminderWindow) (bool, error) {
	if f.reminded == nil {
		f.reminded = map[string]bool{}
	}
	key := fmt.Sprintf("%d/%s", appointmentID, window)
	if f.reminded[key] {
		return false, nil
	}
	f.reminded[key] = true
	return true, nil
}

func (f *fakeAppointmentRepo) Create(ctx context.Context, appointment *models.Appointment) (*models.Appointment, error) {
	f.createInput = appointment
	if f.createErr != nil {
		f.byKey = f.clashWinner
		return nil, f.createErr
	}
	created := *appointment
	created.ID = uint64(50 + len(f.created))
	f.created = append(f.created, &created)
	f.byID = &models.AppointmentDetails{Appointment: created}
	return &created, nil
}

func (f *fakeAppointmentRepo) GetByID(ctx context.Context, id uint64) (*models.AppointmentDetails, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.byID == nil {
		return nil, apperrors.ErrAppointmentNotFound
	}
	found := *f.byID
	return &found, nil
}

func (f *fakeAppointmentRepo) GetByIdempotencyKey(ctx context.Context, userID uint64, key string) (*models.AppointmentDetails, error) {
	if f.byKey == nil || f.byKey.IdempotencyKey != key {
		return nil, apperrors.ErrAppointmentNotFound
	}
	found := *f.byKey
	return &found, nil
}

func (f *fakeAppointmentRepo) UpdateStatus(ctx context.Context, id uint64, fromStatus models.AppointmentStatus, toStatus models.AppointmentStatus) error {
	f.updateStatusTo = toStatus
	f.statusUpdates = append(f.statusUpdates, fmt.Sprintf("%d:%s->%s", id, fromStatus, toStatus))
	return f.updateStatusErr
}

func (f *fakeAppointmentRepo) Cancel(ctx context.Context, id uint64, cancelledByUserID uint64, reason models.CancelReason) error {
	f.cancelCalled = true
	f.cancelReason = reason
	f.cancelledIDs = append(f.cancelledIDs, id)
	return f.cancelErr
}

func (f *fakeAppointmentRepo) ListByUser(ctx context.Context, userID uint64, filter models.AppointmentFilter) ([]*models.AppointmentDetails, error) {
	f.listFilter = filter
	return f.booked, nil
}

func (f *fakeAppointmentRepo) ListByDoctorBetween(ctx context.Context, doctorID uint64, from time.Time, to time.Time) ([]*models.AppointmentDetails, error) {
	f.listFrom = from
	f.listTo = to
	var listed []*models.AppointmentDetails
	for _, appointment := range f.booked {
		if appointment.DoctorID == 0 || appointment.DoctorID == doctorID {
			listed = append(listed, appointment)
		}
	}
	return listed, nil
}

func (f *fakeAppointmentRepo) CreateStatusChange(ctx context.Context, change *models.AppointmentStatusChange) error {
	f.statusChanges = append(f.statusChanges, change)
	return nil
}

func (f *fakeAppointmentRepo) ListPatientsByDoctor(ctx context.Context, doctorID uint64, query string, now time.Time, limit int, offset int) ([]*models.DoctorPatient, error) {
	f.patientsQuery = query
	f.patientsLimit = limit
	return f.patients, nil
}

func (f *fakeAppointmentRepo) HasCountedBooking(ctx context.Context, doctorID uint64, patientID uint64) (bool, error) {
	return f.treated[patientID], nil
}

type fakeOutboxRepo struct {
	events      []*models.OutboxEvent
	err         error
	unpublished []*models.OutboxEvent
	published   []uint64
}

func (f *fakeOutboxRepo) ListUnpublished(ctx context.Context, limit int) ([]*models.OutboxEvent, error) {
	return f.unpublished, nil
}

func (f *fakeOutboxRepo) MarkPublished(ctx context.Context, ids []uint64) error {
	f.published = append(f.published, ids...)
	return nil
}

func (f *fakeOutboxRepo) Create(ctx context.Context, event *models.OutboxEvent) error {
	if f.err != nil {
		return f.err
	}
	f.events = append(f.events, event)
	return nil
}

func (f *fakeAppointmentRepo) MarkPaid(ctx context.Context, id uint64) error {
	f.markPaidCalled = true
	if f.markPaidErr != nil {
		return f.markPaidErr
	}
	if f.byID != nil {
		f.byID.PaymentStatus = models.AppointmentPaymentPaid
	}
	return nil
}

func (f *fakeAppointmentRepo) SetPaymentStatus(ctx context.Context, id uint64, fromStatus models.AppointmentPaymentStatus, toStatus models.AppointmentPaymentStatus) error {
	f.paymentStatusTo = toStatus
	return nil
}

func (f *fakeAppointmentRepo) ListUnpaidDue(ctx context.Context, dueBefore time.Time, limit int) ([]*models.AppointmentDetails, error) {
	return f.unpaidDue, nil
}

func (f *fakeAppointmentRepo) CancelUnpaid(ctx context.Context, id uint64, dueBefore time.Time) error {
	f.cancelUnpaidIDs = append(f.cancelUnpaidIDs, id)
	return f.cancelUnpaidErr
}

type fakePaymentRepo struct {
	payment   *models.Payment
	createErr error
	updateErr error
	movedFrom uint64
	movedTo   uint64
	changes   []*models.PaymentStatusChange
	refunds   []*models.Refund
	refundErr error
	finished  []models.RefundStatus

	listedStatus   models.RefundStatus
	listedLimit    int
	listedDoctorID uint64
}

func (f *fakePaymentRepo) Create(ctx context.Context, payment *models.Payment) (*models.Payment, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	created := *payment
	created.ID = 70
	f.payment = &created
	found := created
	return &found, nil
}

func (f *fakePaymentRepo) GetByID(ctx context.Context, id uint64) (*models.Payment, error) {
	if f.payment == nil || f.payment.ID != id {
		return nil, apperrors.ErrPaymentNotFound
	}
	found := *f.payment
	return &found, nil
}

func (f *fakePaymentRepo) GetLatestByAppointment(ctx context.Context, appointmentID uint64) (*models.Payment, error) {
	if f.payment == nil {
		return nil, apperrors.ErrPaymentNotFound
	}
	found := *f.payment
	return &found, nil
}

func (f *fakePaymentRepo) GetActiveByAppointment(ctx context.Context, appointmentID uint64) (*models.Payment, error) {
	if f.payment == nil || (f.payment.AppointmentID != 0 && f.payment.AppointmentID != appointmentID) ||
		(f.payment.Status != models.PaymentStatusCreated && f.payment.Status != models.PaymentStatusCaptured && f.payment.Status != models.PaymentStatusClaimed) {
		return nil, apperrors.ErrPaymentNotFound
	}
	found := *f.payment
	return &found, nil
}

func (f *fakePaymentRepo) UpdateStatus(ctx context.Context, id uint64, fromStatus models.PaymentStatus, toStatus models.PaymentStatus, providerPaymentID string, capturedAt *time.Time) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	if f.payment == nil || f.payment.Status != fromStatus {
		return apperrors.ErrPaymentNotModifiable
	}
	f.payment.Status = toStatus
	if providerPaymentID != "" {
		f.payment.ProviderPaymentID = providerPaymentID
	}
	return nil
}

func (f *fakePaymentRepo) MoveToAppointment(ctx context.Context, fromAppointmentID uint64, toAppointmentID uint64) error {
	f.movedFrom = fromAppointmentID
	f.movedTo = toAppointmentID
	return nil
}

func (f *fakePaymentRepo) CreateStatusChange(ctx context.Context, change *models.PaymentStatusChange) error {
	f.changes = append(f.changes, change)
	return nil
}

func (f *fakePaymentRepo) CreateRefund(ctx context.Context, refund *models.Refund) (*models.Refund, error) {
	if f.refundErr != nil {
		return nil, f.refundErr
	}
	created := *refund
	created.ID = 90
	f.refunds = append(f.refunds, &created)
	return &created, nil
}

func (f *fakePaymentRepo) FinishRefund(ctx context.Context, id uint64, status models.RefundStatus, providerRefundID string) error {
	for _, refund := range f.refunds {
		if refund.ID == id {
			if refund.Status != models.RefundStatusRequested {
				return apperrors.ErrPaymentNotModifiable
			}
			refund.Status = status
			refund.ProviderRefundID = providerRefundID
		}
	}
	f.finished = append(f.finished, status)
	return nil
}

func (f *fakePaymentRepo) GetByProviderPaymentID(ctx context.Context, provider string, providerPaymentID string) (*models.Payment, error) {
	if f.payment == nil || f.payment.ProviderPaymentID != providerPaymentID {
		return nil, apperrors.ErrPaymentNotFound
	}
	found := *f.payment
	return &found, nil
}

func (f *fakePaymentRepo) GetRequestedRefundByPayment(ctx context.Context, paymentID uint64) (*models.Refund, error) {
	for _, refund := range f.refunds {
		if refund.PaymentID == paymentID && refund.Status == models.RefundStatusRequested {
			found := *refund
			return &found, nil
		}
	}
	return nil, apperrors.ErrRefundNotFound
}

func (f *fakePaymentRepo) GetByProviderOrderID(ctx context.Context, provider string, providerOrderID string) (*models.Payment, error) {
	if f.payment == nil || f.payment.ProviderOrderID != providerOrderID {
		return nil, apperrors.ErrPaymentNotFound
	}
	found := *f.payment
	return &found, nil
}

func (f *fakePaymentRepo) GetRefundByID(ctx context.Context, id uint64) (*models.Refund, error) {
	for _, refund := range f.refunds {
		if refund.ID == id {
			found := *refund
			return &found, nil
		}
	}
	return nil, apperrors.ErrRefundNotFound
}

func (f *fakePaymentRepo) ListRefunds(ctx context.Context, status models.RefundStatus, limit int, offset int) ([]*models.Refund, error) {
	f.listedStatus = status
	f.listedLimit = limit
	var listed []*models.Refund
	for _, refund := range f.refunds {
		if refund.Status == status {
			listed = append(listed, refund)
		}
	}
	return listed, nil
}

type fakeGateway struct {
	orders    int
	verifyErr error
	paidAt    time.Time
	refundErr error
	// refundStatus is what Refund answers; empty means succeeded.
	refundStatus models.RefundStatus
	refunded     int
	paidOrder    string
	findErr      error
	asked        int
	// gatewayRefundID and gatewayRefundStatus are what RefundStatus answers.
	gatewayRefundID     string
	gatewayRefundStatus models.RefundStatus
	refundStatusErr     error
	event               *models.GatewayEvent
	parseErr            error
}

func (f *fakeGateway) Provider() string {
	return "fake"
}

func (f *fakeGateway) CreateOrder(ctx context.Context, appointmentID uint64, amount int, currency string) (string, error) {
	f.orders++
	return fmt.Sprintf("order-%d", f.orders), nil
}

func (f *fakeGateway) ClientPayload(payment *models.Payment) map[string]any {
	return map[string]any{"orderId": payment.ProviderOrderID}
}

func (f *fakeGateway) VerifyPayment(ctx context.Context, payment *models.Payment, providerPaymentID string, signature string) (time.Time, error) {
	return f.paidAt, f.verifyErr
}

func (f *fakeGateway) FindPaidOrder(ctx context.Context, payment *models.Payment) (string, error) {
	f.asked++
	return f.paidOrder, f.findErr
}

func (f *fakeGateway) Refund(ctx context.Context, payment *models.Payment, refund *models.Refund) (string, models.RefundStatus, error) {
	if f.refundErr != nil {
		return "", "", f.refundErr
	}
	f.refunded += refund.Amount
	if f.refundStatus == "" {
		return "refund-1", models.RefundStatusSucceeded, nil
	}
	return "refund-1", f.refundStatus, nil
}

func (f *fakeGateway) RefundStatus(ctx context.Context, payment *models.Payment) (string, models.RefundStatus, error) {
	return f.gatewayRefundID, f.gatewayRefundStatus, f.refundStatusErr
}

func (f *fakeGateway) ParseWebhook(body []byte, headers map[string][]string) (*models.GatewayEvent, error) {
	if f.parseErr != nil {
		return nil, f.parseErr
	}
	return f.event, nil
}

func (f *fakeAppointmentRepo) CreateProposal(ctx context.Context, proposal *models.RescheduleProposal) (*models.RescheduleProposal, error) {
	created := *proposal
	created.ID = 80
	f.createdProposal = &created
	f.proposal = &created
	return &created, nil
}

func (f *fakeAppointmentRepo) GetPendingProposal(ctx context.Context, appointmentID uint64) (*models.RescheduleProposal, error) {
	if f.proposal == nil || f.proposal.AppointmentID != appointmentID {
		return nil, apperrors.ErrProposalNotFound
	}
	found := *f.proposal
	return &found, nil
}

func (f *fakeAppointmentRepo) ListExpiredProposals(ctx context.Context, now time.Time, limit int) ([]*models.RescheduleProposal, error) {
	return f.expiredProposals, nil
}

func (f *fakeAppointmentRepo) CloseProposal(ctx context.Context, id uint64, toStatus models.ProposalStatus, respondedByUserID uint64) error {
	if f.closeErr != nil {
		return f.closeErr
	}
	f.closedTo = toStatus
	f.closedBy = respondedByUserID
	return nil
}

type fakeMedicalRecordRepo struct {
	record      *models.MedicalRecord
	createErr   error
	created     bool
	updated     *models.MedicalRecord
	finalizeErr error

	listed []*models.MedicalRecord

	attachments         []*models.MedicalAttachment
	createAttachmentErr error

	accesses  []*models.RecordAccess
	accessLog []*models.RecordAccess

	viewDoctorID uint64
}

func (f *fakeMedicalRecordRepo) Create(ctx context.Context, record *models.MedicalRecord) (*models.MedicalRecord, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	created := *record
	created.ID = 90
	created.Status = models.MedicalRecordStatusDraft
	f.record = &created
	f.created = true
	found := created
	return &found, nil
}

func (f *fakeMedicalRecordRepo) GetByID(ctx context.Context, id uint64) (*models.MedicalRecord, error) {
	if f.record == nil || f.record.ID != id {
		return nil, apperrors.ErrMedicalRecordNotFound
	}
	found := *f.record
	return &found, nil
}

func (f *fakeMedicalRecordRepo) GetByAppointment(ctx context.Context, appointmentID uint64) (*models.MedicalRecord, error) {
	if f.record == nil || f.record.AppointmentID != appointmentID {
		return nil, apperrors.ErrMedicalRecordNotFound
	}
	found := *f.record
	return &found, nil
}

func (f *fakeMedicalRecordRepo) UpdateDraft(ctx context.Context, record *models.MedicalRecord) error {
	if f.record == nil || f.record.Status != models.MedicalRecordStatusDraft {
		return apperrors.ErrMedicalRecordFinalized
	}
	updated := *record
	updated.PatientID = f.record.PatientID
	updated.DoctorID = f.record.DoctorID
	updated.AppointmentID = f.record.AppointmentID
	updated.Status = f.record.Status
	f.updated = &updated
	f.record = &updated
	return nil
}

func (f *fakeMedicalRecordRepo) Finalize(ctx context.Context, id uint64) error {
	if f.finalizeErr != nil {
		return f.finalizeErr
	}
	f.record.Status = models.MedicalRecordStatusFinalized
	return nil
}

func (f *fakeMedicalRecordRepo) ListFinalizedByPatient(ctx context.Context, patientID uint64, limit int, offset int) ([]*models.MedicalRecord, error) {
	return f.listed, nil
}

func (f *fakeMedicalRecordRepo) ListForDoctorView(ctx context.Context, patientID uint64, doctorID uint64, limit int, offset int) ([]*models.MedicalRecord, error) {
	f.viewDoctorID = doctorID
	return f.listed, nil
}

func (f *fakeMedicalRecordRepo) CreateAttachment(ctx context.Context, attachment *models.MedicalAttachment) (*models.MedicalAttachment, error) {
	if f.createAttachmentErr != nil {
		return nil, f.createAttachmentErr
	}
	created := *attachment
	created.ID = uint64(95 + len(f.attachments))
	f.attachments = append(f.attachments, &created)
	return &created, nil
}

func (f *fakeMedicalRecordRepo) GetAttachment(ctx context.Context, id uint64) (*models.MedicalAttachment, error) {
	for _, attachment := range f.attachments {
		if attachment.ID == id {
			found := *attachment
			return &found, nil
		}
	}
	return nil, apperrors.ErrAttachmentNotFound
}

func (f *fakeMedicalRecordRepo) ListAttachmentsByPatient(ctx context.Context, patientID uint64, limit int, offset int) ([]*models.MedicalAttachment, error) {
	return f.attachments, nil
}

func (f *fakeMedicalRecordRepo) ListAttachmentsByRecord(ctx context.Context, recordID uint64) ([]*models.MedicalAttachment, error) {
	return f.attachments, nil
}

func (f *fakeMedicalRecordRepo) LogAccess(ctx context.Context, access *models.RecordAccess) error {
	f.accesses = append(f.accesses, access)
	return nil
}

func (f *fakeMedicalRecordRepo) ListAccessLog(ctx context.Context, recordID uint64, limit int, offset int) ([]*models.RecordAccess, error) {
	return f.accessLog, nil
}

type fakeFileStorage struct {
	files   map[string][]byte
	deleted []string
	putErr  error
}

func (f *fakeFileStorage) Put(ctx context.Context, key string, content io.Reader, contentType string) error {
	if f.putErr != nil {
		return f.putErr
	}
	data, err := io.ReadAll(content)
	if err != nil {
		return err
	}
	if f.files == nil {
		f.files = map[string][]byte{}
	}
	f.files[key] = data
	return nil
}

func (f *fakeFileStorage) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	data, ok := f.files[key]
	if !ok {
		return nil, apperrors.ErrStoredFileNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (f *fakeFileStorage) Delete(ctx context.Context, key string) error {
	delete(f.files, key)
	f.deleted = append(f.deleted, key)
	return nil
}

func liveCallRoom(room *models.CallRoom) bool {
	return room.Status == models.CallRoomStatusOpen || room.Status == models.CallRoomStatusActive
}

type fakeCallRoomRepo struct {
	rooms     []*models.CallRoom
	createErr error
}

func (f *fakeCallRoomRepo) Create(ctx context.Context, room *models.CallRoom) (*models.CallRoom, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	for _, existing := range f.rooms {
		if !liveCallRoom(existing) {
			continue
		}
		if room.AppointmentID != nil && existing.AppointmentID != nil && *room.AppointmentID == *existing.AppointmentID {
			return nil, apperrors.ErrCallRoomExists
		}
		if room.JoinCodeHash != "" && room.JoinCodeHash == existing.JoinCodeHash {
			return nil, apperrors.ErrCallJoinCodeExists
		}
	}
	created := *room
	created.ID = uint64(len(f.rooms) + 1)
	created.CreatedAt = room.OpenedAt
	f.rooms = append(f.rooms, &created)
	stored := created
	return &stored, nil
}

func (f *fakeCallRoomRepo) find(match func(*models.CallRoom) bool) (*models.CallRoom, error) {
	for _, room := range f.rooms {
		if match(room) {
			found := *room
			return &found, nil
		}
	}
	return nil, apperrors.ErrCallRoomNotFound
}

func (f *fakeCallRoomRepo) GetByID(ctx context.Context, id uint64) (*models.CallRoom, error) {
	return f.find(func(room *models.CallRoom) bool { return room.ID == id })
}

func (f *fakeCallRoomRepo) GetByIDForUpdate(ctx context.Context, id uint64) (*models.CallRoom, error) {
	return f.GetByID(ctx, id)
}

func (f *fakeCallRoomRepo) GetLiveByAppointment(ctx context.Context, appointmentID uint64) (*models.CallRoom, error) {
	return f.find(func(room *models.CallRoom) bool {
		return liveCallRoom(room) && room.AppointmentID != nil && *room.AppointmentID == appointmentID
	})
}

func (f *fakeCallRoomRepo) GetLiveByAppointmentForUpdate(ctx context.Context, appointmentID uint64) (*models.CallRoom, error) {
	return f.GetLiveByAppointment(ctx, appointmentID)
}

func (f *fakeCallRoomRepo) GetLiveByJoinCodeHashForUpdate(ctx context.Context, joinCodeHash string) (*models.CallRoom, error) {
	return f.find(func(room *models.CallRoom) bool {
		return liveCallRoom(room) && room.JoinCodeHash == joinCodeHash
	})
}

func (f *fakeCallRoomRepo) ListByHost(ctx context.Context, hostUserID uint64, kind models.CallRoomKind, limit int) ([]*models.CallRoom, error) {
	var rooms []*models.CallRoom
	for i := len(f.rooms) - 1; i >= 0 && len(rooms) < limit; i-- {
		if f.rooms[i].HostUserID == hostUserID && f.rooms[i].Kind == kind {
			found := *f.rooms[i]
			rooms = append(rooms, &found)
		}
	}
	return rooms, nil
}

func (f *fakeCallRoomRepo) ListLive(ctx context.Context, limit int) ([]*models.CallRoom, error) {
	var rooms []*models.CallRoom
	for _, room := range f.rooms {
		if liveCallRoom(room) && len(rooms) < limit {
			found := *room
			rooms = append(rooms, &found)
		}
	}
	return rooms, nil
}

func (f *fakeCallRoomRepo) CountLiveByHost(ctx context.Context, hostUserID uint64, kind models.CallRoomKind) (int64, error) {
	var count int64
	for _, room := range f.rooms {
		if room.HostUserID == hostUserID && room.Kind == kind && liveCallRoom(room) {
			count++
		}
	}
	return count, nil
}

func (f *fakeCallRoomRepo) CountCreatedByHostSince(ctx context.Context, hostUserID uint64, kind models.CallRoomKind, since time.Time) (int64, error) {
	var count int64
	for _, room := range f.rooms {
		if room.HostUserID == hostUserID && room.Kind == kind && room.CreatedAt.After(since) {
			count++
		}
	}
	return count, nil
}

func (f *fakeCallRoomRepo) MarkActive(ctx context.Context, id uint64, startedAt time.Time) (bool, error) {
	for _, room := range f.rooms {
		if room.ID == id && room.Status == models.CallRoomStatusOpen {
			room.Status = models.CallRoomStatusActive
			room.StartedAt = &startedAt
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeCallRoomRepo) End(ctx context.Context, id uint64, reason models.CallEndReason, endedAt time.Time) error {
	for _, room := range f.rooms {
		if room.ID == id && liveCallRoom(room) {
			room.Status = models.CallRoomStatusEnded
			room.EndReason = reason
			room.EndedAt = &endedAt
			return nil
		}
	}
	return apperrors.ErrCallRoomNotFound
}

type fakeCallParticipantRepo struct {
	participants []*models.CallParticipant
}

func (f *fakeCallParticipantRepo) Create(ctx context.Context, participant *models.CallParticipant) (*models.CallParticipant, error) {
	created := *participant
	created.ID = uint64(len(f.participants) + 1)
	f.participants = append(f.participants, &created)
	stored := created
	return &stored, nil
}

func (f *fakeCallParticipantRepo) GetByID(ctx context.Context, id uint64) (*models.CallParticipant, error) {
	for _, participant := range f.participants {
		if participant.ID == id {
			found := *participant
			return &found, nil
		}
	}
	return nil, apperrors.ErrCallParticipantNotFound
}

func (f *fakeCallParticipantRepo) ListByRoom(ctx context.Context, roomID uint64) ([]*models.CallParticipant, error) {
	var participants []*models.CallParticipant
	for _, participant := range f.participants {
		if participant.CallRoomID == roomID {
			found := *participant
			participants = append(participants, &found)
		}
	}
	return participants, nil
}

func (f *fakeCallParticipantRepo) Update(ctx context.Context, participant *models.CallParticipant) (*models.CallParticipant, error) {
	for i, existing := range f.participants {
		if existing.ID == participant.ID {
			updated := *participant
			f.participants[i] = &updated
			stored := updated
			return &stored, nil
		}
	}
	return nil, apperrors.ErrCallParticipantNotFound
}

func (f *fakeCallParticipantRepo) Touch(ctx context.Context, id uint64, seenAt time.Time) error {
	for _, participant := range f.participants {
		if participant.ID == id {
			participant.LastSeenAt = &seenAt
		}
	}
	return nil
}

// connect marks a participant as connected at now, as its socket does.
func (f *fakeCallParticipantRepo) connect(id uint64, now time.Time) {
	for _, participant := range f.participants {
		if participant.ID == id {
			participant.JoinedAt = &now
			participant.LastSeenAt = &now
			participant.LeftAt = nil
		}
	}
}

type fakeCallGuestTokenRepo struct {
	tokens []*models.CallGuestToken
}

func (f *fakeCallGuestTokenRepo) Create(ctx context.Context, guestToken *models.CallGuestToken) (*models.CallGuestToken, error) {
	created := *guestToken
	created.ID = uint64(len(f.tokens) + 1)
	f.tokens = append(f.tokens, &created)
	stored := created
	return &stored, nil
}

func (f *fakeCallGuestTokenRepo) GetByTokenHashForUpdate(ctx context.Context, tokenHash string) (*models.CallGuestToken, error) {
	for _, guestToken := range f.tokens {
		if guestToken.TokenHash == tokenHash {
			found := *guestToken
			return &found, nil
		}
	}
	return nil, apperrors.ErrCallGuestLinkNotFound
}

func (f *fakeCallGuestTokenRepo) IncrementUse(ctx context.Context, id uint64) error {
	for _, guestToken := range f.tokens {
		if guestToken.ID == id {
			guestToken.UseCount++
		}
	}
	return nil
}

func (f *fakeCallGuestTokenRepo) Revoke(ctx context.Context, appointmentID uint64, id uint64, revokedAt time.Time) error {
	for _, guestToken := range f.tokens {
		if guestToken.ID == id && guestToken.AppointmentID == appointmentID && guestToken.RevokedAt == nil {
			guestToken.RevokedAt = &revokedAt
			return nil
		}
	}
	return apperrors.ErrCallGuestLinkNotFound
}

type fakeCallEventRepo struct {
	events []*models.CallEvent
}

func (f *fakeCallEventRepo) Create(ctx context.Context, event *models.CallEvent) error {
	f.events = append(f.events, event)
	return nil
}

func (f *fakeCallEventRepo) count(eventType models.CallEventType) int {
	count := 0
	for _, event := range f.events {
		if event.EventType == eventType {
			count++
		}
	}
	return count
}

type fakeCallJoinAttempt struct {
	clientIP string
	at       time.Time
}

type fakeCallJoinAttemptRepo struct {
	attempts []fakeCallJoinAttempt
}

func (f *fakeCallJoinAttemptRepo) Create(ctx context.Context, clientIP string, at time.Time) error {
	f.attempts = append(f.attempts, fakeCallJoinAttempt{clientIP: clientIP, at: at})
	return nil
}

func (f *fakeCallJoinAttemptRepo) CountSince(ctx context.Context, clientIP string, since time.Time) (int64, error) {
	var count int64
	for _, attempt := range f.attempts {
		if attempt.clientIP == clientIP && attempt.at.After(since) {
			count++
		}
	}
	return count, nil
}

func (f *fakeCallJoinAttemptRepo) DeleteBefore(ctx context.Context, clientIP string, before time.Time) error {
	kept := f.attempts[:0]
	for _, attempt := range f.attempts {
		if attempt.clientIP != clientIP || attempt.at.After(before) {
			kept = append(kept, attempt)
		}
	}
	f.attempts = kept
	return nil
}

type fakeCallPeer struct {
	mu      sync.Mutex
	peerID  string
	signals []models.CallSignal
	closed  bool
}

func (f *fakeCallPeer) PeerID() string {
	return f.peerID
}

func (f *fakeCallPeer) Send(signal models.CallSignal) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.signals = append(f.signals, signal)
	return nil
}

func (f *fakeCallPeer) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
}

// received returns the signals of signalType the peer got, oldest first.
func (f *fakeCallPeer) received(signalType models.CallSignalType) []models.CallSignal {
	f.mu.Lock()
	defer f.mu.Unlock()
	var signals []models.CallSignal
	for _, signal := range f.signals {
		if signal.Type == signalType {
			signals = append(signals, signal)
		}
	}
	return signals
}

func (f *fakeDoctorRepo) UpdateUPI(ctx context.Context, id uint64, upiID string, qrStorageKey *string) error {
	f.upiID = upiID
	f.upiQRKey = qrStorageKey
	for _, doctor := range []*models.Doctor{f.byUserID, f.byID} {
		if doctor != nil {
			doctor.UPIID = upiID
			doctor.UPIQRStorageKey = qrStorageKey
		}
	}
	return nil
}

func (f *fakeAppointmentRepo) SetPaymentDue(ctx context.Context, id uint64, fromStatus models.AppointmentPaymentStatus, toStatus models.AppointmentPaymentStatus, dueAt *time.Time) error {
	if f.byID != nil && (f.byID.PaymentStatus != fromStatus || f.byID.Status != models.AppointmentStatusScheduled) {
		return apperrors.ErrAppointmentNotModifiable
	}
	f.paymentStatusTo = toStatus
	f.paymentDueAt = dueAt
	if f.byID != nil {
		f.byID.PaymentStatus = toStatus
		f.byID.PaymentDueAt = dueAt
	}
	return nil
}

func (f *fakeAppointmentRepo) CancelForRejectedPayment(ctx context.Context, id uint64, actingUserID uint64) error {
	if f.byID != nil && (f.byID.PaymentStatus != models.AppointmentPaymentPendingVerification || f.byID.Status != models.AppointmentStatusScheduled) {
		return apperrors.ErrAppointmentNotModifiable
	}
	f.cancelledIDs = append(f.cancelledIDs, id)
	f.cancelReason = models.CancelReasonPaymentRejected
	if f.byID != nil {
		f.byID.Status = models.AppointmentStatusCancelled
		f.byID.PaymentStatus = models.AppointmentPaymentUnpaid
		f.byID.PaymentDueAt = nil
	}
	return nil
}

func (f *fakePaymentRepo) Claim(ctx context.Context, id uint64, reference string, proofStorageKey *string, claimedAt time.Time) error {
	if f.payment == nil || f.payment.ID != id || f.payment.Status != models.PaymentStatusCreated {
		return apperrors.ErrPaymentNotModifiable
	}
	f.payment.Status = models.PaymentStatusClaimed
	f.payment.PayerReference = reference
	f.payment.ProofStorageKey = proofStorageKey
	f.payment.ClaimedAt = &claimedAt
	return nil
}

func (f *fakePaymentRepo) Review(ctx context.Context, id uint64, toStatus models.PaymentStatus, reviewedByUserID uint64, note string, reviewedAt time.Time) error {
	if f.payment == nil || f.payment.ID != id || f.payment.Status != models.PaymentStatusClaimed {
		return apperrors.ErrPaymentNotModifiable
	}
	f.payment.Status = toStatus
	f.payment.ReviewedByUserID = reviewedByUserID
	f.payment.ReviewNote = note
	f.payment.ReviewedAt = &reviewedAt
	return nil
}

func (f *fakePaymentRepo) ListClaimed(ctx context.Context, doctorID uint64, limit int, offset int) ([]*models.Payment, error) {
	f.listedDoctorID = doctorID
	if f.payment == nil || f.payment.Status != models.PaymentStatusClaimed {
		return []*models.Payment{}, nil
	}
	found := *f.payment
	return []*models.Payment{&found}, nil
}

func (f *fakePaymentRepo) ListManualRefunds(ctx context.Context, doctorID uint64, status models.RefundStatus, limit int, offset int) ([]*models.Refund, error) {
	f.listedDoctorID = doctorID
	refunds := []*models.Refund{}
	for _, refund := range f.refunds {
		if refund.Status == status {
			found := *refund
			refunds = append(refunds, &found)
		}
	}
	return refunds, nil
}
