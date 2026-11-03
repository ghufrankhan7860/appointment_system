package handler_test

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
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

func (f *fakeUserRepo) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	return nil, apperrors.ErrInvalidCredentials
}

func (f *fakeUserRepo) FindByMobile(ctx context.Context, mobileNo string) (*models.User, error) {
	return nil, apperrors.ErrInvalidCredentials
}

func (f *fakeUserRepo) GetByID(ctx context.Context, id uint64) (*models.User, error) {
	return nil, apperrors.ErrUserNotFound
}

func (f *fakeUserRepo) ExistsByRole(ctx context.Context, role models.Role) (bool, error) {
	return true, nil
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
	if f.payment == nil || (f.payment.Status != models.PaymentStatusCreated && f.payment.Status != models.PaymentStatusCaptured) {
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
