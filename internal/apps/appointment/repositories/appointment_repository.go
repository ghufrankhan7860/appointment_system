// Package repositories contains appointment persistence adapters.
//
// Implement only persistence behavior required by a consuming service contract. Pass
// context to every query, parameterize values, bound result sets, translate records into
// domain models, and classify database errors. Accept a narrow transaction/query object
// when needed. Do not enforce business eligibility, call providers, or publish events.
package repositories

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AppointmentRepository struct {
	db *gorm.DB
}

func NewAppointmentRepository(db *gorm.DB) (*AppointmentRepository, error) {
	if db == nil {
		return nil, apperrors.ErrNilDB
	}
	return &AppointmentRepository{db: db}, nil
}

type appointmentRecord struct {
	ID                uint64     `gorm:"primaryKey;autoIncrement;column:id"`
	PatientID         uint64     `gorm:"column:patient_id"`
	DoctorID          uint64     `gorm:"column:doctor_id"`
	BookedByUserID    uint64     `gorm:"column:booked_by_user_id"`
	StartsAt          time.Time  `gorm:"column:starts_at"`
	EndsAt            time.Time  `gorm:"column:ends_at"`
	Mode              string     `gorm:"column:mode"`
	Status            string     `gorm:"column:status"`
	ConsultationFee   int        `gorm:"column:consultation_fee"`
	SlotDurationMin   int        `gorm:"column:slot_duration_min"`
	PatientNotes      string     `gorm:"column:patient_notes"`
	CancelledAt       *time.Time `gorm:"column:cancelled_at"`
	CancelledByUserID *uint64    `gorm:"column:cancelled_by_user_id"`
	CancelReason      *string    `gorm:"column:cancel_reason"`
	RescheduledFromID *uint64    `gorm:"column:rescheduled_from_id"`
	IdempotencyKey    *string    `gorm:"column:idempotency_key"`
	PaymentStatus     string     `gorm:"column:payment_status"`
	PaymentDueAt      *time.Time `gorm:"column:payment_due_at"`
	CreatedAt         time.Time  `gorm:"column:created_at"`
	UpdatedAt         time.Time  `gorm:"column:updated_at"`
}

func (appointmentRecord) TableName() string {
	return "appointments"
}

type appointmentDetailsRecord struct {
	Appointment          appointmentRecord `gorm:"embedded"`
	DoctorName           string            `gorm:"column:doctor_name"`
	DoctorSpeciality     string            `gorm:"column:doctor_speciality"`
	PatientName          string            `gorm:"column:patient_name"`
	BookedByRelationship *string           `gorm:"column:booked_by_relationship"`
}

type appointmentStatusChangeRecord struct {
	ID              uint64    `gorm:"primaryKey;autoIncrement;column:id"`
	AppointmentID   uint64    `gorm:"column:appointment_id"`
	ChangedByUserID *uint64   `gorm:"column:changed_by_user_id"`
	FromStatus      string    `gorm:"column:from_status"`
	ToStatus        string    `gorm:"column:to_status"`
	Reason          string    `gorm:"column:reason"`
	CreatedAt       time.Time `gorm:"column:created_at"`
}

func (appointmentStatusChangeRecord) TableName() string {
	return "appointment_status_history"
}

type appointmentReminderRecord struct {
	AppointmentID  uint64    `gorm:"primaryKey;column:appointment_id"`
	ReminderWindow string    `gorm:"primaryKey;column:reminder_window"`
	CreatedAt      time.Time `gorm:"column:created_at"`
}

func (appointmentReminderRecord) TableName() string {
	return "appointment_reminders"
}

type rescheduleProposalRecord struct {
	ID                       uint64     `gorm:"primaryKey;autoIncrement;column:id"`
	AppointmentID            uint64     `gorm:"column:appointment_id"`
	PlaceholderAppointmentID uint64     `gorm:"column:placeholder_appointment_id"`
	ProposedByUserID         uint64     `gorm:"column:proposed_by_user_id"`
	Reason                   string     `gorm:"column:reason"`
	Note                     string     `gorm:"column:note"`
	Status                   string     `gorm:"column:status"`
	ExpiresAt                time.Time  `gorm:"column:expires_at"`
	RespondedAt              *time.Time `gorm:"column:responded_at"`
	RespondedByUserID        *uint64    `gorm:"column:responded_by_user_id"`
	CreatedAt                time.Time  `gorm:"column:created_at"`
	UpdatedAt                time.Time  `gorm:"column:updated_at"`
}

func (rescheduleProposalRecord) TableName() string {
	return "appointment_reschedule_proposals"
}

type rescheduleProposalDetailsRecord struct {
	Proposal    rescheduleProposalRecord `gorm:"embedded"`
	NewStartsAt time.Time                `gorm:"column:new_starts_at"`
	NewEndsAt   time.Time                `gorm:"column:new_ends_at"`
	NewMode     string                   `gorm:"column:new_mode"`
}

// Create inserts an appointment. The appointments_doctor_no_overlap constraint
// rejects a second scheduled appointment in the same doctor time, so two
// concurrent bookings of one slot cannot both succeed. A repeated
// (booked_by_user_id, idempotency_key) pair returns ErrIdempotencyKeyExists.
func (repo *AppointmentRepository) Create(ctx context.Context, appointment *models.Appointment) (*models.Appointment, error) {
	if appointment == nil {
		return nil, apperrors.ErrNilAppointmentRequest
	}

	now := time.Now().UTC()
	record := appointmentRecord{
		PatientID:         appointment.PatientID,
		DoctorID:          appointment.DoctorID,
		BookedByUserID:    appointment.BookedByUserID,
		StartsAt:          appointment.StartsAt.UTC(),
		EndsAt:            appointment.EndsAt.UTC(),
		Mode:              string(appointment.Mode),
		Status:            string(appointment.Status),
		ConsultationFee:   appointment.ConsultationFee,
		SlotDurationMin:   appointment.SlotDurationMin,
		PatientNotes:      appointment.PatientNotes,
		RescheduledFromID: appointment.RescheduledFromID,
		PaymentStatus:     string(appointment.PaymentStatus),
		PaymentDueAt:      appointment.PaymentDueAt,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if appointment.IdempotencyKey != "" {
		record.IdempotencyKey = &appointment.IdempotencyKey
	}

	if err := dbFromContext(ctx, repo.db).Create(&record).Error; err != nil {
		if doctorNoOverlapExclusion.ViolatedBy(err) {
			return nil, apperrors.ErrSlotTaken
		}
		if appointmentIdempotencyKeyUnique.ViolatedBy(err) {
			return nil, apperrors.ErrIdempotencyKeyExists
		}
		return nil, fmt.Errorf("create appointment record: %w", err)
	}

	return toAppointmentModel(record), nil
}

func (repo *AppointmentRepository) GetByID(ctx context.Context, id uint64) (*models.AppointmentDetails, error) {
	var record appointmentDetailsRecord

	if err := repo.detailsQuery(ctx).
		Where("appointments.id = ?", id).
		Take(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrAppointmentNotFound
		}
		return nil, fmt.Errorf("get appointment by id: %w", err)
	}

	return toAppointmentDetailsModel(record), nil
}

// MarkPaid records that a scheduled, unpaid appointment was paid. It only
// matches a row that is still both, so a payment and an expiry cannot both win.
func (repo *AppointmentRepository) MarkPaid(ctx context.Context, id uint64) error {
	result := dbFromContext(ctx, repo.db).
		Model(&appointmentRecord{}).
		Where("id = ? AND status = ? AND payment_status = ?", id, string(models.AppointmentStatusScheduled), string(models.AppointmentPaymentUnpaid)).
		Updates(map[string]interface{}{
			"payment_status": string(models.AppointmentPaymentPaid),
			"updated_at":     time.Now().UTC(),
		})
	if result.Error != nil {
		return fmt.Errorf("mark appointment paid: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrAppointmentNotModifiable
	}

	return nil
}

// SetPaymentStatus moves an appointment's payment status from one value to
// another, whatever its clinical status.
func (repo *AppointmentRepository) SetPaymentStatus(
	ctx context.Context,
	id uint64,
	fromStatus models.AppointmentPaymentStatus,
	toStatus models.AppointmentPaymentStatus,
) error {
	result := dbFromContext(ctx, repo.db).
		Model(&appointmentRecord{}).
		Where("id = ? AND payment_status = ?", id, string(fromStatus)).
		Updates(map[string]interface{}{
			"payment_status": string(toStatus),
			"updated_at":     time.Now().UTC(),
		})
	if result.Error != nil {
		return fmt.Errorf("set appointment payment status: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrAppointmentNotModifiable
	}

	return nil
}

// ListUnpaidDue returns scheduled, unpaid appointments whose payment was due
// at or before dueBefore, oldest due first.
func (repo *AppointmentRepository) ListUnpaidDue(ctx context.Context, dueBefore time.Time, limit int) ([]*models.AppointmentDetails, error) {
	var records []appointmentDetailsRecord

	if err := repo.detailsQuery(ctx).
		Where("appointments.status = ? AND appointments.payment_status = ? AND appointments.payment_due_at <= ?",
			string(models.AppointmentStatusScheduled), string(models.AppointmentPaymentUnpaid), dueBefore.UTC()).
		Order("appointments.payment_due_at ASC, appointments.id ASC").
		Limit(limit).
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list unpaid appointments due: %w", err)
	}

	return toAppointmentDetailsModels(records), nil
}

// CancelUnpaid cancels a scheduled, unpaid appointment whose payment was due at
// or before dueBefore, as a system change with reason payment_expired. It only
// matches a row that is still unpaid, so a payment and an expiry cannot both win.
func (repo *AppointmentRepository) CancelUnpaid(ctx context.Context, id uint64, dueBefore time.Time) error {
	now := time.Now().UTC()
	result := dbFromContext(ctx, repo.db).
		Model(&appointmentRecord{}).
		Where("id = ? AND status = ? AND payment_status = ? AND payment_due_at <= ?",
			id, string(models.AppointmentStatusScheduled), string(models.AppointmentPaymentUnpaid), dueBefore.UTC()).
		Updates(map[string]interface{}{
			"status":        string(models.AppointmentStatusCancelled),
			"cancelled_at":  now,
			"cancel_reason": string(models.CancelReasonPaymentExpired),
			"updated_at":    now,
		})
	if result.Error != nil {
		return fmt.Errorf("cancel unpaid appointment: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrAppointmentNotModifiable
	}

	return nil
}

// GetByIdempotencyKey returns the appointment the user created with key.
func (repo *AppointmentRepository) GetByIdempotencyKey(ctx context.Context, userID uint64, key string) (*models.AppointmentDetails, error) {
	var record appointmentDetailsRecord

	if err := repo.detailsQuery(ctx).
		Where("appointments.booked_by_user_id = ? AND appointments.idempotency_key = ?", userID, key).
		Take(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrAppointmentNotFound
		}
		return nil, fmt.Errorf("get appointment by idempotency key: %w", err)
	}

	return toAppointmentDetailsModel(record), nil
}

// UpdateStatus moves an appointment from one status to another. It only matches
// a row still in fromStatus, so two concurrent changes cannot both win.
func (repo *AppointmentRepository) UpdateStatus(
	ctx context.Context,
	id uint64,
	fromStatus models.AppointmentStatus,
	toStatus models.AppointmentStatus,
) error {
	result := dbFromContext(ctx, repo.db).
		Model(&appointmentRecord{}).
		Where("id = ? AND status = ?", id, string(fromStatus)).
		Updates(map[string]interface{}{
			"status":     string(toStatus),
			"updated_at": time.Now().UTC(),
		})
	if result.Error != nil {
		return fmt.Errorf("update appointment status: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrAppointmentNotModifiable
	}

	return nil
}

// Cancel moves a scheduled appointment, or one held by a reschedule proposal,
// to cancelled. It only matches a row still in one of those statuses, so a
// cancel cannot overwrite a concurrent change. A cancelledByUserID of 0 is a
// change the system made.
func (repo *AppointmentRepository) Cancel(
	ctx context.Context,
	id uint64,
	cancelledByUserID uint64,
	reason models.CancelReason,
) error {
	now := time.Now().UTC()
	result := dbFromContext(ctx, repo.db).
		Model(&appointmentRecord{}).
		Where("id = ? AND status IN ?", id, []string{string(models.AppointmentStatusScheduled), string(models.AppointmentStatusRescheduleProposed)}).
		Updates(map[string]interface{}{
			"status":               string(models.AppointmentStatusCancelled),
			"cancelled_at":         now,
			"cancelled_by_user_id": optionalUserID(cancelledByUserID),
			"cancel_reason":        string(reason),
			"updated_at":           now,
		})
	if result.Error != nil {
		return fmt.Errorf("cancel appointment: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrAppointmentNotModifiable
	}

	return nil
}

// ListByUser returns appointments of every patient the user has an active link
// to, earliest first.
func (repo *AppointmentRepository) ListByUser(
	ctx context.Context,
	userID uint64,
	filter models.AppointmentFilter,
) ([]*models.AppointmentDetails, error) {
	var records []appointmentDetailsRecord

	linkedPatients := dbFromContext(ctx, repo.db).
		Model(&userPatientRecord{}).
		Select("patient_id").
		Where("user_id = ? AND status = ?", userID, string(models.UserPatientStatusActive))

	query := repo.detailsQuery(ctx).Where("appointments.patient_id IN (?)", linkedPatients)
	if filter.PatientID != 0 {
		query = query.Where("appointments.patient_id = ?", filter.PatientID)
	}
	if filter.Status != "" {
		query = query.Where("appointments.status = ?", string(filter.Status))
	}
	if !filter.From.IsZero() {
		query = query.Where("appointments.starts_at >= ?", filter.From.UTC())
	}
	if !filter.To.IsZero() {
		query = query.Where("appointments.starts_at < ?", filter.To.UTC())
	}

	if err := query.
		Order("appointments.starts_at ASC, appointments.id ASC").
		Limit(filter.Limit).
		Offset(filter.Offset).
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list appointments by user: %w", err)
	}

	return toAppointmentDetailsModels(records), nil
}

// ListByDoctorBetween returns the doctor's appointments in any status that start
// in [from, to), earliest first.
func (repo *AppointmentRepository) ListByDoctorBetween(
	ctx context.Context,
	doctorID uint64,
	from time.Time,
	to time.Time,
) ([]*models.AppointmentDetails, error) {
	var records []appointmentDetailsRecord

	if err := repo.detailsQuery(ctx).
		Where("appointments.doctor_id = ? AND appointments.starts_at >= ? AND appointments.starts_at < ?", doctorID, from.UTC(), to.UTC()).
		Order("appointments.starts_at ASC, appointments.id ASC").
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list appointments by doctor: %w", err)
	}

	return toAppointmentDetailsModels(records), nil
}

func (repo *AppointmentRepository) CreateStatusChange(ctx context.Context, change *models.AppointmentStatusChange) error {
	if change == nil {
		return apperrors.ErrNilAppointmentRequest
	}

	record := appointmentStatusChangeRecord{
		AppointmentID:   change.AppointmentID,
		ChangedByUserID: optionalUserID(change.ChangedByUserID),
		FromStatus:      string(change.FromStatus),
		ToStatus:        string(change.ToStatus),
		Reason:          change.Reason,
		CreatedAt:       time.Now().UTC(),
	}

	if err := dbFromContext(ctx, repo.db).Create(&record).Error; err != nil {
		return fmt.Errorf("create appointment status change record: %w", err)
	}

	return nil
}

// ListDueForReminder returns paid or free scheduled appointments starting in
// [from, to) that have not had the window's reminder yet, earliest first.
func (repo *AppointmentRepository) ListDueForReminder(
	ctx context.Context,
	window models.ReminderWindow,
	from time.Time,
	to time.Time,
	limit int,
) ([]*models.AppointmentDetails, error) {
	var records []appointmentDetailsRecord

	if err := repo.detailsQuery(ctx).
		Where("appointments.status = ? AND appointments.starts_at >= ? AND appointments.starts_at < ?", string(models.AppointmentStatusScheduled), from.UTC(), to.UTC()).
		Where("appointments.payment_status IN ?", []string{string(models.AppointmentPaymentPaid), string(models.AppointmentPaymentWaived)}).
		Where("NOT EXISTS (SELECT 1 FROM appointment_reminders WHERE appointment_reminders.appointment_id = appointments.id AND appointment_reminders.reminder_window = ?)", string(window)).
		Order("appointments.starts_at ASC, appointments.id ASC").
		Limit(limit).
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list appointments due for reminder: %w", err)
	}

	return toAppointmentDetailsModels(records), nil
}

// CreateReminder records that the window's reminder was queued. It reports
// false when it was already recorded, so a reminder is never queued twice.
func (repo *AppointmentRepository) CreateReminder(ctx context.Context, appointmentID uint64, window models.ReminderWindow) (bool, error) {
	record := appointmentReminderRecord{
		AppointmentID:  appointmentID,
		ReminderWindow: string(window),
		CreatedAt:      time.Now().UTC(),
	}

	result := dbFromContext(ctx, repo.db).Clauses(clause.OnConflict{DoNothing: true}).Create(&record)
	if result.Error != nil {
		return false, fmt.Errorf("create appointment reminder record: %w", result.Error)
	}

	return result.RowsAffected == 1, nil
}

// detailsQuery selects appointments with the doctor name, primary speciality,
// patient name and the booker's relationship to the patient.
func (repo *AppointmentRepository) detailsQuery(ctx context.Context) *gorm.DB {
	return dbFromContext(ctx, repo.db).
		Table("appointments").
		Select(`appointments.*,
			doctors.full_name AS doctor_name,
			patients.name AS patient_name,
			booked_by.patient_relationship AS booked_by_relationship,
			COALESCE((
				SELECT specialities.name
				FROM doctor_specialities
				JOIN specialities ON specialities.id = doctor_specialities.speciality_id
				WHERE doctor_specialities.doctor_id = appointments.doctor_id AND doctor_specialities.is_primary
				LIMIT 1
			), '') AS doctor_speciality`).
		Joins("JOIN doctors ON doctors.id = appointments.doctor_id").
		Joins("JOIN patients ON patients.id = appointments.patient_id").
		Joins("LEFT JOIN user_patient booked_by ON booked_by.user_id = appointments.booked_by_user_id AND booked_by.patient_id = appointments.patient_id")
}

func toAppointmentModel(record appointmentRecord) *models.Appointment {
	appointment := &models.Appointment{
		ID:                record.ID,
		PatientID:         record.PatientID,
		DoctorID:          record.DoctorID,
		BookedByUserID:    record.BookedByUserID,
		StartsAt:          record.StartsAt,
		EndsAt:            record.EndsAt,
		Mode:              models.ConsultationMode(record.Mode),
		Status:            models.AppointmentStatus(record.Status),
		ConsultationFee:   record.ConsultationFee,
		SlotDurationMin:   record.SlotDurationMin,
		PatientNotes:      record.PatientNotes,
		CancelledAt:       record.CancelledAt,
		CancelledByUserID: record.CancelledByUserID,
		RescheduledFromID: record.RescheduledFromID,
		PaymentStatus:     models.AppointmentPaymentStatus(record.PaymentStatus),
		PaymentDueAt:      record.PaymentDueAt,
		CreatedAt:         record.CreatedAt,
		UpdatedAt:         record.UpdatedAt,
	}
	if record.CancelReason != nil {
		appointment.CancelReason = models.CancelReason(*record.CancelReason)
	}
	if record.IdempotencyKey != nil {
		appointment.IdempotencyKey = *record.IdempotencyKey
	}

	return appointment
}

func toAppointmentDetailsModel(record appointmentDetailsRecord) *models.AppointmentDetails {
	details := &models.AppointmentDetails{
		Appointment:      *toAppointmentModel(record.Appointment),
		DoctorName:       record.DoctorName,
		DoctorSpeciality: record.DoctorSpeciality,
		PatientName:      record.PatientName,
	}
	if record.BookedByRelationship != nil {
		details.BookedByRelationship = models.Relationship(*record.BookedByRelationship)
	}

	return details
}

// CreateProposal records a pending reschedule proposal. An appointment can have
// only one pending proposal; a second one returns ErrAppointmentNotModifiable.
func (repo *AppointmentRepository) CreateProposal(ctx context.Context, proposal *models.RescheduleProposal) (*models.RescheduleProposal, error) {
	if proposal == nil {
		return nil, apperrors.ErrNilAppointmentRequest
	}

	now := time.Now().UTC()
	record := rescheduleProposalRecord{
		AppointmentID:            proposal.AppointmentID,
		PlaceholderAppointmentID: proposal.PlaceholderAppointmentID,
		ProposedByUserID:         proposal.ProposedByUserID,
		Reason:                   string(proposal.Reason),
		Note:                     proposal.Note,
		Status:                   string(proposal.Status),
		ExpiresAt:                proposal.ExpiresAt.UTC(),
		CreatedAt:                now,
		UpdatedAt:                now,
	}

	if err := dbFromContext(ctx, repo.db).Create(&record).Error; err != nil {
		if pendingProposalUnique.ViolatedBy(err) {
			return nil, apperrors.ErrAppointmentNotModifiable
		}
		return nil, fmt.Errorf("create reschedule proposal record: %w", err)
	}

	return toRescheduleProposalModel(rescheduleProposalDetailsRecord{Proposal: record}), nil
}

// GetPendingProposal returns the appointment's pending proposal with the
// offered time read from its placeholder.
func (repo *AppointmentRepository) GetPendingProposal(ctx context.Context, appointmentID uint64) (*models.RescheduleProposal, error) {
	var record rescheduleProposalDetailsRecord

	if err := repo.proposalQuery(ctx).
		Where("appointment_reschedule_proposals.appointment_id = ? AND appointment_reschedule_proposals.status = ?", appointmentID, string(models.ProposalStatusPending)).
		Take(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrProposalNotFound
		}
		return nil, fmt.Errorf("get pending reschedule proposal: %w", err)
	}

	return toRescheduleProposalModel(record), nil
}

// ListExpiredProposals returns pending proposals whose answer was due at or
// before now, oldest first.
func (repo *AppointmentRepository) ListExpiredProposals(ctx context.Context, now time.Time, limit int) ([]*models.RescheduleProposal, error) {
	var records []rescheduleProposalDetailsRecord

	if err := repo.proposalQuery(ctx).
		Where("appointment_reschedule_proposals.status = ? AND appointment_reschedule_proposals.expires_at <= ?", string(models.ProposalStatusPending), now.UTC()).
		Order("appointment_reschedule_proposals.expires_at ASC, appointment_reschedule_proposals.id ASC").
		Limit(limit).
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list expired reschedule proposals: %w", err)
	}

	result := make([]*models.RescheduleProposal, 0, len(records))
	for _, record := range records {
		result = append(result, toRescheduleProposalModel(record))
	}

	return result, nil
}

// CloseProposal answers a pending proposal. It only matches a proposal that is
// still pending, so an answer and an expiry cannot both win. A
// respondedByUserID of 0 is the system.
func (repo *AppointmentRepository) CloseProposal(
	ctx context.Context,
	id uint64,
	toStatus models.ProposalStatus,
	respondedByUserID uint64,
) error {
	now := time.Now().UTC()
	result := dbFromContext(ctx, repo.db).
		Model(&rescheduleProposalRecord{}).
		Where("id = ? AND status = ?", id, string(models.ProposalStatusPending)).
		Updates(map[string]interface{}{
			"status":               string(toStatus),
			"responded_at":         now,
			"responded_by_user_id": optionalUserID(respondedByUserID),
			"updated_at":           now,
		})
	if result.Error != nil {
		return fmt.Errorf("close reschedule proposal: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrAppointmentNotModifiable
	}

	return nil
}

func (repo *AppointmentRepository) proposalQuery(ctx context.Context) *gorm.DB {
	return dbFromContext(ctx, repo.db).
		Table("appointment_reschedule_proposals").
		Select(`appointment_reschedule_proposals.*,
			placeholder.starts_at AS new_starts_at,
			placeholder.ends_at AS new_ends_at,
			placeholder.mode AS new_mode`).
		Joins("JOIN appointments placeholder ON placeholder.id = appointment_reschedule_proposals.placeholder_appointment_id")
}

func toRescheduleProposalModel(record rescheduleProposalDetailsRecord) *models.RescheduleProposal {
	proposal := &models.RescheduleProposal{
		ID:                       record.Proposal.ID,
		AppointmentID:            record.Proposal.AppointmentID,
		PlaceholderAppointmentID: record.Proposal.PlaceholderAppointmentID,
		ProposedByUserID:         record.Proposal.ProposedByUserID,
		Reason:                   models.ProposalReason(record.Proposal.Reason),
		Note:                     record.Proposal.Note,
		Status:                   models.ProposalStatus(record.Proposal.Status),
		ExpiresAt:                record.Proposal.ExpiresAt,
		RespondedAt:              record.Proposal.RespondedAt,
		NewStartsAt:              record.NewStartsAt,
		NewEndsAt:                record.NewEndsAt,
		NewMode:                  models.ConsultationMode(record.NewMode),
		CreatedAt:                record.Proposal.CreatedAt,
		UpdatedAt:                record.Proposal.UpdatedAt,
	}
	if record.Proposal.RespondedByUserID != nil {
		proposal.RespondedByUserID = *record.Proposal.RespondedByUserID
	}

	return proposal
}

// optionalUserID stores 0, meaning a change the system made, as NULL.
func optionalUserID(userID uint64) *uint64 {
	if userID == 0 {
		return nil
	}

	return &userID
}

func toAppointmentDetailsModels(records []appointmentDetailsRecord) []*models.AppointmentDetails {
	result := make([]*models.AppointmentDetails, 0, len(records))
	for _, record := range records {
		result = append(result, toAppointmentDetailsModel(record))
	}

	return result
}
