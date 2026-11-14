package handler_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
)

// discardLogger satisfies the services and handlers that log.
var discardLogger = slog.New(slog.DiscardHandler)

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
	userPatient  *models.UserPatient
	patientLinks []*models.UserPatient
	createErr    error
	getErr       error
}

func (f *fakeUserPatientRepo) ListActiveByPatient(ctx context.Context, patientID uint64) ([]*models.UserPatient, error) {
	return f.patientLinks, nil
}

func (f *fakeUserPatientRepo) ListActiveByPatientForUpdate(ctx context.Context, patientID uint64) ([]*models.UserPatient, error) {
	return f.patientLinks, nil
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
	return &models.UserPatient{UserID: userID, PatientID: patientID, Status: models.UserPatientStatusRevoked}, nil
}

type fakeTxManager struct{}

func (f *fakeTxManager) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type fakeTokenVerifier struct{}

func (f *fakeTokenVerifier) Verify(token string) (uint64, string, uint64, error) {
	if token != "good-token" {
		return 0, "", 0, errors.New("invalid token")
	}
	return 7, "patient", 21, nil
}

type fakeRoleTokenVerifier struct{}

func (f *fakeRoleTokenVerifier) Verify(token string) (uint64, string, uint64, error) {
	switch token {
	case "admin-token":
		return 1, "admin", 31, nil
	case "doctor-token":
		return 11, "doctor", 32, nil
	case "patient-token":
		return 7, "patient", 33, nil
	case "staff-token":
		return 40, "staff", 34, nil
	case "unlinked-staff-token":
		return 41, "staff", 35, nil
	default:
		return 0, "", 0, errors.New("invalid token")
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

func (f *fakeUserRepo) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	return nil, apperrors.ErrInvalidCredentials
}

func (f *fakeUserRepo) FindByMobile(ctx context.Context, mobileNo string) (*models.User, error) {
	return nil, apperrors.ErrInvalidCredentials
}

// GetByID knows only the patient user 7.
func (f *fakeUserRepo) GetByID(ctx context.Context, id uint64) (*models.User, error) {
	if id == 7 {
		return &models.User{ID: 7, Role: models.RolePatient}, nil
	}
	if id == 11 {
		return &models.User{ID: 11, Name: "Dr. Rao", Role: models.RoleDoctor}, nil
	}
	return nil, apperrors.ErrUserNotFound
}

func (f *fakeUserRepo) ExistsByRole(ctx context.Context, role models.Role) (bool, error) {
	return true, nil
}

func (f *fakeUserRepo) GetByIDs(ctx context.Context, ids []uint64) (map[uint64]*models.User, error) {
	return map[uint64]*models.User{
		7: {ID: 7, Name: "Meera", MobileNo: "9876543210"},
		8: {ID: 8, Name: "Ravi", MobileNo: "9123456780"},
	}, nil
}

type fakePatientInviteRepo struct {
	invite  *models.PatientInvite
	getErr  error
	pending int64
}

func (f *fakePatientInviteRepo) Create(ctx context.Context, invite *models.PatientInvite) (*models.PatientInvite, error) {
	created := *invite
	created.ID = 31
	return &created, nil
}

func (f *fakePatientInviteRepo) GetByCodeHashForUpdate(ctx context.Context, codeHash string) (*models.PatientInvite, error) {
	return f.invite, f.getErr
}

func (f *fakePatientInviteRepo) ListPendingByPatient(ctx context.Context, patientID uint64, now time.Time) ([]*models.PatientInvite, error) {
	return []*models.PatientInvite{}, nil
}

func (f *fakePatientInviteRepo) CountPendingByPatient(ctx context.Context, patientID uint64, now time.Time) (int64, error) {
	return f.pending, nil
}

func (f *fakePatientInviteRepo) MarkRedeemed(ctx context.Context, id uint64, redeemedByUserID uint64, redeemedAt time.Time) error {
	return nil
}

func (f *fakePatientInviteRepo) Revoke(ctx context.Context, patientID uint64, id uint64) error {
	return f.getErr
}

func (f *fakePatientInviteRepo) MarkExpired(ctx context.Context, id uint64) error {
	return nil
}

type fakeInviteAttemptRepo struct {
	count int64
}

func (f *fakeInviteAttemptRepo) Create(ctx context.Context, userID uint64, at time.Time) error {
	return nil
}

func (f *fakeInviteAttemptRepo) CountSince(ctx context.Context, userID uint64, since time.Time) (int64, error) {
	return f.count, nil
}

func (f *fakeInviteAttemptRepo) DeleteBefore(ctx context.Context, userID uint64, before time.Time) error {
	return nil
}

type fakeUserPatientHistoryRepo struct{}

func (f *fakeUserPatientHistoryRepo) Create(ctx context.Context, history *models.UserPatientHistory) error {
	return nil
}

type fakeSessionRepo struct {
	sessions      []*models.Session
	revokeErr     error
	revokedCount  int64
	revokedUserID uint64
	revokedID     uint64
	exceptID      uint64
}

func (f *fakeSessionRepo) Create(ctx context.Context, session *models.Session) error {
	session.ID = 41
	return nil
}

func (f *fakeSessionRepo) GetByTokenHash(ctx context.Context, tokenHash string) (*models.Session, error) {
	return nil, apperrors.ErrSessionNotFound
}

func (f *fakeSessionRepo) RevokeByID(ctx context.Context, id uint64) error {
	return nil
}

func (f *fakeSessionRepo) Rotate(ctx context.Context, id uint64, oldTokenHash string, session *models.Session) error {
	return nil
}

func (f *fakeSessionRepo) ListActiveByUser(ctx context.Context, userID uint64, now time.Time) ([]*models.Session, error) {
	return f.sessions, nil
}

func (f *fakeSessionRepo) ListByUser(ctx context.Context, userID uint64, limit int, offset int) ([]*models.Session, error) {
	return f.sessions, nil
}

func (f *fakeSessionRepo) RevokeByUserAndID(ctx context.Context, userID uint64, id uint64) error {
	f.revokedUserID = userID
	f.revokedID = id
	return f.revokeErr
}

func (f *fakeSessionRepo) RevokeActiveByUser(ctx context.Context, userID uint64, exceptID uint64) (int64, error) {
	f.revokedUserID = userID
	f.exceptID = exceptID
	return f.revokedCount, nil
}

func (f *fakeSessionRepo) DeleteEndedBefore(ctx context.Context, before time.Time, limit int) (int64, error) {
	return 0, nil
}

type fakeUserAgentParser struct{}

func (f *fakeUserAgentParser) Parse(userAgent string) models.UserAgentDetails {
	return models.UserAgentDetails{}
}

type fakeAppointmentRepo struct {
	booked    []*models.AppointmentDetails
	byID      *models.AppointmentDetails
	createErr error
	proposal  *models.RescheduleProposal
	patients  []*models.DoctorPatient
	treated   map[uint64]bool
}

func (f *fakeAppointmentRepo) Create(ctx context.Context, appointment *models.Appointment) (*models.Appointment, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	created := *appointment
	created.ID = 50
	f.byID = &models.AppointmentDetails{Appointment: created, PatientName: "Asha Verma", BookedByRelationship: models.RelationshipChild}
	return &created, nil
}

func (f *fakeAppointmentRepo) GetByID(ctx context.Context, id uint64) (*models.AppointmentDetails, error) {
	if f.byID == nil || f.byID.ID != id {
		return nil, apperrors.ErrAppointmentNotFound
	}
	found := *f.byID
	return &found, nil
}

func (f *fakeAppointmentRepo) GetByIdempotencyKey(ctx context.Context, userID uint64, key string) (*models.AppointmentDetails, error) {
	return nil, apperrors.ErrAppointmentNotFound
}

func (f *fakeAppointmentRepo) UpdateStatus(ctx context.Context, id uint64, fromStatus models.AppointmentStatus, toStatus models.AppointmentStatus) error {
	f.byID.Status = toStatus
	return nil
}

func (f *fakeAppointmentRepo) Cancel(ctx context.Context, id uint64, cancelledByUserID uint64, reason models.CancelReason) error {
	f.byID.Status = models.AppointmentStatusCancelled
	f.byID.CancelReason = reason
	return nil
}

func (f *fakeAppointmentRepo) ListByUser(ctx context.Context, userID uint64, filter models.AppointmentFilter) ([]*models.AppointmentDetails, error) {
	return f.booked, nil
}

func (f *fakeAppointmentRepo) ListByDoctorBetween(ctx context.Context, doctorID uint64, from time.Time, to time.Time) ([]*models.AppointmentDetails, error) {
	return f.booked, nil
}

func (f *fakeAppointmentRepo) ListPatientsByDoctor(ctx context.Context, doctorID uint64, query string, now time.Time, limit int, offset int) ([]*models.DoctorPatient, error) {
	return f.patients, nil
}

func (f *fakeAppointmentRepo) HasCountedBooking(ctx context.Context, doctorID uint64, patientID uint64) (bool, error) {
	return f.treated[patientID], nil
}

func (f *fakeAppointmentRepo) ListDueForReminder(ctx context.Context, window models.ReminderWindow, from time.Time, to time.Time, limit int) ([]*models.AppointmentDetails, error) {
	return nil, nil
}

func (f *fakeAppointmentRepo) CreateReminder(ctx context.Context, appointmentID uint64, window models.ReminderWindow) (bool, error) {
	return true, nil
}

func (f *fakeAppointmentRepo) CreateStatusChange(ctx context.Context, change *models.AppointmentStatusChange) error {
	return nil
}

type fakeOutboxRepo struct{}

func (f *fakeOutboxRepo) ListUnpublished(ctx context.Context, limit int) ([]*models.OutboxEvent, error) {
	return nil, nil
}

func (f *fakeOutboxRepo) MarkPublished(ctx context.Context, ids []uint64) error {
	return nil
}

func (f *fakeOutboxRepo) Create(ctx context.Context, event *models.OutboxEvent) error {
	return nil
}

func (f *fakeAppointmentRepo) MarkPaid(ctx context.Context, id uint64) error {
	f.byID.PaymentStatus = models.AppointmentPaymentPaid
	return nil
}

func (f *fakeAppointmentRepo) SetPaymentStatus(ctx context.Context, id uint64, fromStatus models.AppointmentPaymentStatus, toStatus models.AppointmentPaymentStatus) error {
	return nil
}

func (f *fakeAppointmentRepo) ListUnpaidDue(ctx context.Context, dueBefore time.Time, limit int) ([]*models.AppointmentDetails, error) {
	return nil, nil
}

func (f *fakeAppointmentRepo) CancelUnpaid(ctx context.Context, id uint64, dueBefore time.Time) error {
	return nil
}

type fakePaymentRepo struct {
	payment *models.Payment
	refunds []*models.Refund
}

func (f *fakePaymentRepo) Create(ctx context.Context, payment *models.Payment) (*models.Payment, error) {
	created := *payment
	created.ID = 70
	f.payment = &created
	return &created, nil
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
	if f.payment == nil || (f.payment.Status != models.PaymentStatusCreated && f.payment.Status != models.PaymentStatusCaptured && f.payment.Status != models.PaymentStatusClaimed) {
		return nil, apperrors.ErrPaymentNotFound
	}
	found := *f.payment
	return &found, nil
}

func (f *fakePaymentRepo) UpdateStatus(ctx context.Context, id uint64, fromStatus models.PaymentStatus, toStatus models.PaymentStatus, providerPaymentID string, capturedAt *time.Time) error {
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
	return nil
}

func (f *fakePaymentRepo) CreateStatusChange(ctx context.Context, change *models.PaymentStatusChange) error {
	return nil
}

func (f *fakePaymentRepo) CreateRefund(ctx context.Context, refund *models.Refund) (*models.Refund, error) {
	created := *refund
	created.ID = 90
	return &created, nil
}

func (f *fakePaymentRepo) FinishRefund(ctx context.Context, id uint64, status models.RefundStatus, providerRefundID string) error {
	for _, refund := range f.refunds {
		if refund.ID == id {
			refund.Status = status
			refund.ProviderRefundID = providerRefundID
		}
	}
	return nil
}

func (f *fakePaymentRepo) GetByProviderOrderID(ctx context.Context, provider string, providerOrderID string) (*models.Payment, error) {
	if f.payment == nil || f.payment.ProviderOrderID != providerOrderID {
		return nil, apperrors.ErrPaymentNotFound
	}
	found := *f.payment
	return &found, nil
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
	var listed []*models.Refund
	for _, refund := range f.refunds {
		if refund.Status == status {
			listed = append(listed, refund)
		}
	}
	return listed, nil
}

func (f *fakeAppointmentRepo) CreateProposal(ctx context.Context, proposal *models.RescheduleProposal) (*models.RescheduleProposal, error) {
	created := *proposal
	created.ID = 80
	if f.byID != nil {
		created.NewStartsAt = f.byID.StartsAt
		created.NewEndsAt = f.byID.EndsAt
		created.NewMode = f.byID.Mode
	}
	f.proposal = &created
	return &created, nil
}

func (f *fakeAppointmentRepo) GetPendingProposal(ctx context.Context, appointmentID uint64) (*models.RescheduleProposal, error) {
	if f.proposal == nil {
		return nil, apperrors.ErrProposalNotFound
	}
	found := *f.proposal
	return &found, nil
}

func (f *fakeAppointmentRepo) ListExpiredProposals(ctx context.Context, now time.Time, limit int) ([]*models.RescheduleProposal, error) {
	return nil, nil
}

func (f *fakeAppointmentRepo) CloseProposal(ctx context.Context, id uint64, toStatus models.ProposalStatus, respondedByUserID uint64) error {
	if f.proposal == nil || f.proposal.Status != models.ProposalStatusPending {
		return apperrors.ErrAppointmentNotModifiable
	}
	f.proposal.Status = toStatus
	return nil
}

type fakeMedicalRecordRepo struct {
	record      *models.MedicalRecord
	attachments []*models.MedicalAttachment
	accesses    []*models.RecordAccess
}

func (f *fakeMedicalRecordRepo) Create(ctx context.Context, record *models.MedicalRecord) (*models.MedicalRecord, error) {
	created := *record
	created.ID = 90
	created.Status = models.MedicalRecordStatusDraft
	f.record = &created
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
	updated.AppointmentID = f.record.AppointmentID
	updated.PatientID = f.record.PatientID
	updated.DoctorID = f.record.DoctorID
	updated.Status = f.record.Status
	f.record = &updated
	return nil
}

func (f *fakeMedicalRecordRepo) Finalize(ctx context.Context, id uint64) error {
	f.record.Status = models.MedicalRecordStatusFinalized
	return nil
}

func (f *fakeMedicalRecordRepo) ListForDoctorView(ctx context.Context, patientID uint64, doctorID uint64, limit int, offset int) ([]*models.MedicalRecord, error) {
	if f.record == nil || (f.record.Status != models.MedicalRecordStatusFinalized && f.record.DoctorID != doctorID) {
		return []*models.MedicalRecord{}, nil
	}
	return []*models.MedicalRecord{f.record}, nil
}

func (f *fakeMedicalRecordRepo) ListFinalizedByPatient(ctx context.Context, patientID uint64, limit int, offset int) ([]*models.MedicalRecord, error) {
	if f.record == nil || f.record.Status != models.MedicalRecordStatusFinalized {
		return []*models.MedicalRecord{}, nil
	}
	return []*models.MedicalRecord{f.record}, nil
}

func (f *fakeMedicalRecordRepo) CreateAttachment(ctx context.Context, attachment *models.MedicalAttachment) (*models.MedicalAttachment, error) {
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
	return f.accesses, nil
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
	if f.doctor == nil || f.doctor.ID != id {
		return apperrors.ErrDoctorNotFound
	}
	f.doctor.UPIID = upiID
	f.doctor.UPIQRStorageKey = qrStorageKey
	return nil
}

func (f *fakeAppointmentRepo) SetPaymentDue(ctx context.Context, id uint64, fromStatus models.AppointmentPaymentStatus, toStatus models.AppointmentPaymentStatus, dueAt *time.Time) error {
	if f.byID != nil {
		if f.byID.PaymentStatus != fromStatus || f.byID.Status != models.AppointmentStatusScheduled {
			return apperrors.ErrAppointmentNotModifiable
		}
		f.byID.PaymentStatus = toStatus
		f.byID.PaymentDueAt = dueAt
	}
	return nil
}

func (f *fakeAppointmentRepo) CancelForRejectedPayment(ctx context.Context, id uint64, actingUserID uint64) error {
	if f.byID != nil {
		if f.byID.PaymentStatus != models.AppointmentPaymentPendingVerification || f.byID.Status != models.AppointmentStatusScheduled {
			return apperrors.ErrAppointmentNotModifiable
		}
		f.byID.Status = models.AppointmentStatusCancelled
		f.byID.CancelReason = models.CancelReasonPaymentRejected
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
	if f.payment == nil || f.payment.Status != models.PaymentStatusClaimed {
		return []*models.Payment{}, nil
	}
	found := *f.payment
	return []*models.Payment{&found}, nil
}

func (f *fakePaymentRepo) ListManualRefunds(ctx context.Context, doctorID uint64, status models.RefundStatus, limit int, offset int) ([]*models.Refund, error) {
	refunds := []*models.Refund{}
	for _, refund := range f.refunds {
		if refund.Status == status {
			found := *refund
			refunds = append(refunds, &found)
		}
	}
	return refunds, nil
}

// fakeFileStorage keeps uploaded payment images in memory.
type fakeFileStorage struct {
	files map[string][]byte
}

func (f *fakeFileStorage) Put(ctx context.Context, key string, content io.Reader, contentType string) error {
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
	return nil
}

// newTestClinicActor resolves desk users against doctors, with no staff and no
// clinic patients; staff tests build their own. A nil doctors (constructor
// tests) gets an empty fake, so the service under test reports its own nil.
func newTestClinicActor(doctors services.DoctorRepository) *services.ClinicActor {
	if doctors == nil {
		doctors = &fakeDoctorRepo{}
	}
	actor, err := services.NewClinicActor(doctors, &fakeStaffRepo{}, &fakeClinicPatientRepo{})
	if err != nil {
		panic(err)
	}
	return actor
}

type fakeStaffRepo struct {
	active    map[uint64]*models.DoctorStaff
	members   []*models.StaffMember
	createErr error
	removed   []uint64
}

func (f *fakeStaffRepo) Create(ctx context.Context, staff *models.DoctorStaff) (*models.DoctorStaff, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	if f.active == nil {
		f.active = map[uint64]*models.DoctorStaff{}
	}
	if _, ok := f.active[staff.UserID]; ok {
		return nil, apperrors.ErrStaffAlreadyLinked
	}
	created := *staff
	created.ID = uint64(len(f.active) + 1)
	created.Status = models.StaffStatusActive
	created.CreatedAt = time.Now()
	f.active[staff.UserID] = &created
	return &created, nil
}

func (f *fakeStaffRepo) GetActiveByUser(ctx context.Context, userID uint64) (*models.DoctorStaff, error) {
	link, ok := f.active[userID]
	if !ok {
		return nil, apperrors.ErrStaffNotFound
	}
	found := *link
	return &found, nil
}

func (f *fakeStaffRepo) ListActiveByDoctor(ctx context.Context, doctorID uint64) ([]*models.StaffMember, error) {
	return f.members, nil
}

func (f *fakeStaffRepo) Remove(ctx context.Context, doctorID uint64, userID uint64, removedByUserID uint64, removedAt time.Time) error {
	link, ok := f.active[userID]
	if !ok || link.DoctorID != doctorID {
		return apperrors.ErrStaffNotFound
	}
	delete(f.active, userID)
	f.removed = append(f.removed, userID)
	return nil
}

type fakeStaffInviteRepo struct {
	invites   []*models.StaffInvite
	createErr []error
	redeemed  []uint64
}

func (f *fakeStaffInviteRepo) Create(ctx context.Context, invite *models.StaffInvite) (*models.StaffInvite, error) {
	if len(f.createErr) > 0 {
		err := f.createErr[0]
		f.createErr = f.createErr[1:]
		if err != nil {
			return nil, err
		}
	}
	created := *invite
	created.ID = uint64(len(f.invites) + 1)
	created.CreatedAt = time.Now()
	f.invites = append(f.invites, &created)
	found := created
	return &found, nil
}

func (f *fakeStaffInviteRepo) GetByCodeHashForUpdate(ctx context.Context, codeHash string) (*models.StaffInvite, error) {
	for _, invite := range f.invites {
		if invite.CodeHash == codeHash {
			found := *invite
			return &found, nil
		}
	}
	return nil, apperrors.ErrStaffInviteNotFound
}

func (f *fakeStaffInviteRepo) ListPendingByDoctor(ctx context.Context, doctorID uint64, now time.Time) ([]*models.StaffInvite, error) {
	pending := []*models.StaffInvite{}
	for _, invite := range f.invites {
		if invite.DoctorID == doctorID && invite.Status == models.StaffInviteStatusPending && invite.ExpiresAt.After(now) {
			pending = append(pending, invite)
		}
	}
	return pending, nil
}

func (f *fakeStaffInviteRepo) CountPendingByDoctor(ctx context.Context, doctorID uint64, now time.Time) (int64, error) {
	pending, _ := f.ListPendingByDoctor(ctx, doctorID, now)
	return int64(len(pending)), nil
}

func (f *fakeStaffInviteRepo) MarkRedeemed(ctx context.Context, id uint64, redeemedByUserID uint64, redeemedAt time.Time) error {
	return f.setStatus(id, models.StaffInviteStatusRedeemed, func(invite *models.StaffInvite) {
		invite.RedeemedByUserID = &redeemedByUserID
		f.redeemed = append(f.redeemed, redeemedByUserID)
	})
}

func (f *fakeStaffInviteRepo) Revoke(ctx context.Context, doctorID uint64, id uint64) error {
	for _, invite := range f.invites {
		if invite.ID == id && invite.DoctorID != doctorID {
			return apperrors.ErrStaffInviteNotFound
		}
	}
	return f.setStatus(id, models.StaffInviteStatusRevoked, nil)
}

func (f *fakeStaffInviteRepo) MarkExpired(ctx context.Context, id uint64) error {
	return f.setStatus(id, models.StaffInviteStatusExpired, nil)
}

func (f *fakeStaffInviteRepo) setStatus(id uint64, status models.StaffInviteStatus, also func(*models.StaffInvite)) error {
	for _, invite := range f.invites {
		if invite.ID == id && invite.Status == models.StaffInviteStatusPending {
			invite.Status = status
			if also != nil {
				also(invite)
			}
			return nil
		}
	}
	return apperrors.ErrStaffInviteNotFound
}

type fakeStaffInviteAttemptRepo struct {
	failed map[string]int64
}

func (f *fakeStaffInviteAttemptRepo) Create(ctx context.Context, clientIP string, at time.Time) error {
	if f.failed == nil {
		f.failed = map[string]int64{}
	}
	f.failed[clientIP]++
	return nil
}

func (f *fakeStaffInviteAttemptRepo) CountSince(ctx context.Context, clientIP string, since time.Time) (int64, error) {
	return f.failed[clientIP], nil
}

func (f *fakeStaffInviteAttemptRepo) DeleteBefore(ctx context.Context, clientIP string, before time.Time) error {
	return nil
}

type fakePatientPassRepo struct {
	passes map[uint64]*models.PatientPass
}

func (f *fakePatientPassRepo) Replace(ctx context.Context, pass *models.PatientPass) (*models.PatientPass, error) {
	if f.passes == nil {
		f.passes = map[uint64]*models.PatientPass{}
	}
	stored := *pass
	stored.ID = pass.PatientID
	f.passes[pass.PatientID] = &stored
	found := stored
	return &found, nil
}

func (f *fakePatientPassRepo) GetByTokenHash(ctx context.Context, tokenHash string) (*models.PatientPass, error) {
	for _, pass := range f.passes {
		if pass.TokenHash == tokenHash {
			found := *pass
			return &found, nil
		}
	}
	return nil, apperrors.ErrPatientPassInvalid
}

func (f *fakePatientPassRepo) GetByCodeHash(ctx context.Context, codeHash string) (*models.PatientPass, error) {
	for _, pass := range f.passes {
		if pass.CodeHash == codeHash {
			found := *pass
			return &found, nil
		}
	}
	return nil, apperrors.ErrPatientPassInvalid
}

type fakePatientPassAttemptRepo struct {
	failed map[uint64]int64
}

func (f *fakePatientPassAttemptRepo) Create(ctx context.Context, userID uint64, at time.Time) error {
	if f.failed == nil {
		f.failed = map[uint64]int64{}
	}
	f.failed[userID]++
	return nil
}

func (f *fakePatientPassAttemptRepo) CountSince(ctx context.Context, userID uint64, since time.Time) (int64, error) {
	return f.failed[userID], nil
}

func (f *fakePatientPassAttemptRepo) DeleteBefore(ctx context.Context, userID uint64, before time.Time) error {
	return nil
}

type fakeClinicPatientRepo struct {
	seen map[[2]uint64]*models.ClinicPatient
}

func (f *fakeClinicPatientRepo) Add(ctx context.Context, clinicPatient *models.ClinicPatient) error {
	if f.seen == nil {
		f.seen = map[[2]uint64]*models.ClinicPatient{}
	}
	key := [2]uint64{clinicPatient.DoctorID, clinicPatient.PatientID}
	if _, ok := f.seen[key]; !ok {
		stored := *clinicPatient
		f.seen[key] = &stored
	}
	return nil
}

func (f *fakeClinicPatientRepo) Get(ctx context.Context, doctorID uint64, patientID uint64) (*models.ClinicPatient, error) {
	found, ok := f.seen[[2]uint64{doctorID, patientID}]
	if !ok {
		return nil, apperrors.ErrPatientNotAtClinic
	}
	return found, nil
}
