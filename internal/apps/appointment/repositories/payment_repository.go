package repositories

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type PaymentRepository struct {
	db *gorm.DB
}

func NewPaymentRepository(db *gorm.DB) (*PaymentRepository, error) {
	if db == nil {
		return nil, apperrors.ErrNilDB
	}
	return &PaymentRepository{db: db}, nil
}

type paymentRecord struct {
	ID                uint64     `gorm:"primaryKey;autoIncrement;column:id"`
	AppointmentID     uint64     `gorm:"column:appointment_id"`
	PayerUserID       uint64     `gorm:"column:payer_user_id"`
	Amount            int        `gorm:"column:amount"`
	Currency          string     `gorm:"column:currency"`
	Provider          string     `gorm:"column:provider"`
	ProviderOrderID   string     `gorm:"column:provider_order_id"`
	ProviderPaymentID *string    `gorm:"column:provider_payment_id"`
	Status            string     `gorm:"column:status"`
	CapturedAt        *time.Time `gorm:"column:captured_at"`
	PayerReference    *string    `gorm:"column:payer_reference"`
	ProofStorageKey   *string    `gorm:"column:proof_storage_key"`
	ClaimedAt         *time.Time `gorm:"column:claimed_at"`
	ReviewedByUserID  *uint64    `gorm:"column:reviewed_by_user_id"`
	ReviewedAt        *time.Time `gorm:"column:reviewed_at"`
	ReviewNote        string     `gorm:"column:review_note"`
	CreatedAt         time.Time  `gorm:"column:created_at"`
	UpdatedAt         time.Time  `gorm:"column:updated_at"`
}

func (paymentRecord) TableName() string {
	return "payments"
}

type paymentStatusChangeRecord struct {
	ID              uint64    `gorm:"primaryKey;autoIncrement;column:id"`
	PaymentID       uint64    `gorm:"column:payment_id"`
	ChangedByUserID *uint64   `gorm:"column:changed_by_user_id"`
	FromStatus      string    `gorm:"column:from_status"`
	ToStatus        string    `gorm:"column:to_status"`
	Reason          string    `gorm:"column:reason"`
	CreatedAt       time.Time `gorm:"column:created_at"`
}

func (paymentStatusChangeRecord) TableName() string {
	return "payment_status_history"
}

type refundRecord struct {
	ID                uint64     `gorm:"primaryKey;autoIncrement;column:id"`
	PaymentID         uint64     `gorm:"column:payment_id"`
	InitiatedByUserID *uint64    `gorm:"column:initiated_by_user_id"`
	Amount            int        `gorm:"column:amount"`
	Reason            string     `gorm:"column:reason"`
	Note              string     `gorm:"column:note"`
	ProviderRefundID  *string    `gorm:"column:provider_refund_id"`
	Status            string     `gorm:"column:status"`
	ProcessedAt       *time.Time `gorm:"column:processed_at"`
	CreatedAt         time.Time  `gorm:"column:created_at"`
	UpdatedAt         time.Time  `gorm:"column:updated_at"`
}

func (refundRecord) TableName() string {
	return "refunds"
}

// Create inserts a payment. An appointment can hold only one live (created or
// captured) payment; a second one returns ErrPaymentAlreadyExists.
func (repo *PaymentRepository) Create(ctx context.Context, payment *models.Payment) (*models.Payment, error) {
	if payment == nil {
		return nil, apperrors.ErrNilPaymentRequest
	}

	now := time.Now().UTC()
	record := paymentRecord{
		AppointmentID:   payment.AppointmentID,
		PayerUserID:     payment.PayerUserID,
		Amount:          payment.Amount,
		Currency:        payment.Currency,
		Provider:        payment.Provider,
		ProviderOrderID: payment.ProviderOrderID,
		Status:          string(payment.Status),
		CapturedAt:      payment.CapturedAt,
		ReviewedByUserID: optionalUserID(payment.ReviewedByUserID),
		ReviewedAt:      payment.ReviewedAt,
		ReviewNote:      payment.ReviewNote,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := dbFromContext(ctx, repo.db).Create(&record).Error; err != nil {
		if livePaymentUnique.ViolatedBy(err) {
			return nil, apperrors.ErrPaymentAlreadyExists
		}
		return nil, fmt.Errorf("create payment record: %w", err)
	}

	return toPaymentModel(record), nil
}

func (repo *PaymentRepository) GetByID(ctx context.Context, id uint64) (*models.Payment, error) {
	var record paymentRecord

	if err := dbFromContext(ctx, repo.db).
		Where("id = ?", id).
		First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrPaymentNotFound
		}
		return nil, fmt.Errorf("get payment by id: %w", err)
	}

	return toPaymentModel(record), nil
}

// GetLatestByAppointment returns the appointment's most recent payment in any
// status.
func (repo *PaymentRepository) GetLatestByAppointment(ctx context.Context, appointmentID uint64) (*models.Payment, error) {
	var record paymentRecord

	if err := dbFromContext(ctx, repo.db).
		Where("appointment_id = ?", appointmentID).
		Order("created_at DESC, id DESC").
		First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrPaymentNotFound
		}
		return nil, fmt.Errorf("get latest payment by appointment: %w", err)
	}

	return toPaymentModel(record), nil
}

// GetActiveByAppointment returns the appointment's live payment, the one that
// is created or captured.
func (repo *PaymentRepository) GetActiveByAppointment(ctx context.Context, appointmentID uint64) (*models.Payment, error) {
	var record paymentRecord

	if err := dbFromContext(ctx, repo.db).
		Where("appointment_id = ? AND status IN ?", appointmentID, []string{string(models.PaymentStatusCreated), string(models.PaymentStatusCaptured), string(models.PaymentStatusClaimed)}).
		First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrPaymentNotFound
		}
		return nil, fmt.Errorf("get active payment by appointment: %w", err)
	}

	return toPaymentModel(record), nil
}

// GetByProviderOrderID finds the payment a gateway callback is about.
func (repo *PaymentRepository) GetByProviderOrderID(ctx context.Context, provider string, providerOrderID string) (*models.Payment, error) {
	var record paymentRecord

	if err := dbFromContext(ctx, repo.db).
		Where("provider = ? AND provider_order_id = ?", provider, providerOrderID).
		First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrPaymentNotFound
		}
		return nil, fmt.Errorf("get payment by provider order id: %w", err)
	}

	return toPaymentModel(record), nil
}

// GetByProviderPaymentID finds the payment a gateway payment id confirmed.
func (repo *PaymentRepository) GetByProviderPaymentID(ctx context.Context, provider string, providerPaymentID string) (*models.Payment, error) {
	var record paymentRecord

	if err := dbFromContext(ctx, repo.db).
		Where("provider = ? AND provider_payment_id = ?", provider, providerPaymentID).
		First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrPaymentNotFound
		}
		return nil, fmt.Errorf("get payment by provider payment id: %w", err)
	}

	return toPaymentModel(record), nil
}

// UpdateStatus moves a payment from one status to another. It only matches a
// row still in fromStatus, so two concurrent changes cannot both win. An empty
// providerPaymentID or a nil capturedAt leaves the stored value. A gateway
// payment id that already confirmed another payment returns ErrProviderPaymentUsed.
func (repo *PaymentRepository) UpdateStatus(
	ctx context.Context,
	id uint64,
	fromStatus models.PaymentStatus,
	toStatus models.PaymentStatus,
	providerPaymentID string,
	capturedAt *time.Time,
) error {
	updates := map[string]interface{}{
		"status":     string(toStatus),
		"updated_at": time.Now().UTC(),
	}
	if providerPaymentID != "" {
		updates["provider_payment_id"] = providerPaymentID
	}
	if capturedAt != nil {
		updates["captured_at"] = capturedAt.UTC()
	}

	result := dbFromContext(ctx, repo.db).
		Model(&paymentRecord{}).
		Where("id = ? AND status = ?", id, string(fromStatus)).
		Updates(updates)
	if result.Error != nil {
		if providerPaymentUnique.ViolatedBy(result.Error) {
			return apperrors.ErrProviderPaymentUsed
		}
		return fmt.Errorf("update payment status: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrPaymentNotModifiable
	}

	return nil
}

// MoveToAppointment points every payment of one appointment at another, used
// when an appointment is rescheduled into a new row.
func (repo *PaymentRepository) MoveToAppointment(ctx context.Context, fromAppointmentID uint64, toAppointmentID uint64) error {
	if err := dbFromContext(ctx, repo.db).
		Model(&paymentRecord{}).
		Where("appointment_id = ?", fromAppointmentID).
		Updates(map[string]interface{}{
			"appointment_id": toAppointmentID,
			"updated_at":     time.Now().UTC(),
		}).Error; err != nil {
		return fmt.Errorf("move payments to appointment: %w", err)
	}

	return nil
}

func (repo *PaymentRepository) CreateStatusChange(ctx context.Context, change *models.PaymentStatusChange) error {
	if change == nil {
		return apperrors.ErrNilPaymentRequest
	}

	record := paymentStatusChangeRecord{
		PaymentID:       change.PaymentID,
		ChangedByUserID: optionalUserID(change.ChangedByUserID),
		FromStatus:      string(change.FromStatus),
		ToStatus:        string(change.ToStatus),
		Reason:          change.Reason,
		CreatedAt:       time.Now().UTC(),
	}

	if err := dbFromContext(ctx, repo.db).Create(&record).Error; err != nil {
		return fmt.Errorf("create payment status change record: %w", err)
	}

	return nil
}

// CreateRefund records a requested refund. A payment can have only one refund
// that has not failed; a second one returns ErrRefundAlreadyExists.
func (repo *PaymentRepository) CreateRefund(ctx context.Context, refund *models.Refund) (*models.Refund, error) {
	if refund == nil {
		return nil, apperrors.ErrNilPaymentRequest
	}

	now := time.Now().UTC()
	record := refundRecord{
		PaymentID:         refund.PaymentID,
		InitiatedByUserID: optionalUserID(refund.InitiatedByUserID),
		Amount:            refund.Amount,
		Reason:            string(refund.Reason),
		Note:              refund.Note,
		Status:            string(refund.Status),
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	if err := dbFromContext(ctx, repo.db).Create(&record).Error; err != nil {
		if paymentRefundUnique.ViolatedBy(err) {
			return nil, apperrors.ErrRefundAlreadyExists
		}
		return nil, fmt.Errorf("create refund record: %w", err)
	}

	return toRefundModel(record), nil
}

// FinishRefund records the gateway's answer for a requested refund.
func (repo *PaymentRepository) FinishRefund(ctx context.Context, id uint64, status models.RefundStatus, providerRefundID string) error {
	now := time.Now().UTC()
	updates := map[string]interface{}{
		"status":       string(status),
		"processed_at": now,
		"updated_at":   now,
	}
	if providerRefundID != "" {
		updates["provider_refund_id"] = providerRefundID
	}

	result := dbFromContext(ctx, repo.db).
		Model(&refundRecord{}).
		Where("id = ? AND status = ?", id, string(models.RefundStatusRequested)).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("finish refund: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrPaymentNotModifiable
	}

	return nil
}

func (repo *PaymentRepository) GetRefundByID(ctx context.Context, id uint64) (*models.Refund, error) {
	var record refundRecord

	if err := dbFromContext(ctx, repo.db).
		Where("id = ?", id).
		First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrRefundNotFound
		}
		return nil, fmt.Errorf("get refund by id: %w", err)
	}

	return toRefundModel(record), nil
}

// GetRequestedRefundByPayment returns the payment's refund still waiting for
// the gateway. A payment has at most one.
func (repo *PaymentRepository) GetRequestedRefundByPayment(ctx context.Context, paymentID uint64) (*models.Refund, error) {
	var record refundRecord

	if err := dbFromContext(ctx, repo.db).
		Where("payment_id = ? AND status = ?", paymentID, string(models.RefundStatusRequested)).
		First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrRefundNotFound
		}
		return nil, fmt.Errorf("get requested refund by payment: %w", err)
	}

	return toRefundModel(record), nil
}

// ListRefunds returns refunds in status, oldest first, so the refund waiting
// longest comes first.
func (repo *PaymentRepository) ListRefunds(ctx context.Context, status models.RefundStatus, limit int, offset int) ([]*models.Refund, error) {
	var records []refundRecord

	if err := dbFromContext(ctx, repo.db).
		Where("status = ?", string(status)).
		Order("created_at ASC, id ASC").
		Limit(limit).
		Offset(offset).
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list refunds: %w", err)
	}

	refunds := make([]*models.Refund, 0, len(records))
	for _, record := range records {
		refunds = append(refunds, toRefundModel(record))
	}

	return refunds, nil
}

// Claim records that the payer says they paid a created direct UPI payment:
// their UPI reference and optional screenshot. It only matches a created
// payment, so a claim and an expiry cannot both win.
func (repo *PaymentRepository) Claim(ctx context.Context, id uint64, reference string, proofStorageKey *string, claimedAt time.Time) error {
	result := dbFromContext(ctx, repo.db).
		Model(&paymentRecord{}).
		Where("id = ? AND status = ?", id, string(models.PaymentStatusCreated)).
		Updates(map[string]interface{}{
			"status":            string(models.PaymentStatusClaimed),
			"payer_reference":   reference,
			"proof_storage_key": proofStorageKey,
			"claimed_at":        claimedAt.UTC(),
			"updated_at":        time.Now().UTC(),
		})
	if result.Error != nil {
		return fmt.Errorf("claim payment: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrPaymentNotModifiable
	}

	return nil
}

// Review closes a claimed payment as captured or rejected and records who
// checked it. It only matches a claimed payment, so two reviews cannot both win.
func (repo *PaymentRepository) Review(
	ctx context.Context,
	id uint64,
	toStatus models.PaymentStatus,
	reviewedByUserID uint64,
	note string,
	reviewedAt time.Time,
) error {
	updates := map[string]interface{}{
		"status":              string(toStatus),
		"reviewed_by_user_id": reviewedByUserID,
		"reviewed_at":         reviewedAt.UTC(),
		"review_note":         note,
		"updated_at":          time.Now().UTC(),
	}
	if toStatus == models.PaymentStatusCaptured {
		updates["captured_at"] = reviewedAt.UTC()
	}

	result := dbFromContext(ctx, repo.db).
		Model(&paymentRecord{}).
		Where("id = ? AND status = ?", id, string(models.PaymentStatusClaimed)).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("review payment: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrPaymentNotModifiable
	}

	return nil
}

// ListClaimed returns direct UPI payments waiting for a check, oldest claim
// first: the doctor's own when doctorID is set, every one when it is 0.
func (repo *PaymentRepository) ListClaimed(ctx context.Context, doctorID uint64, limit int, offset int) ([]*models.Payment, error) {
	var records []paymentRecord

	query := dbFromContext(ctx, repo.db).Where("payments.status = ?", string(models.PaymentStatusClaimed))
	if doctorID != 0 {
		query = query.Joins("JOIN appointments ON appointments.id = payments.appointment_id").
			Where("appointments.doctor_id = ?", doctorID)
	}
	if err := query.
		Order("payments.claimed_at ASC, payments.id ASC").
		Limit(limit).
		Offset(offset).
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list claimed payments: %w", err)
	}

	payments := make([]*models.Payment, 0, len(records))
	for _, record := range records {
		payments = append(payments, toPaymentModel(record))
	}

	return payments, nil
}

// ListManualRefunds returns refunds of direct UPI payments in status, oldest
// first: those the doctor owes when doctorID is set, every one when it is 0.
func (repo *PaymentRepository) ListManualRefunds(ctx context.Context, doctorID uint64, status models.RefundStatus, limit int, offset int) ([]*models.Refund, error) {
	var records []refundRecord

	query := dbFromContext(ctx, repo.db).
		Joins("JOIN payments ON payments.id = refunds.payment_id").
		Where("refunds.status = ? AND payments.provider = ?", string(status), models.ProviderUPIDirect)
	if doctorID != 0 {
		query = query.Joins("JOIN appointments ON appointments.id = payments.appointment_id").
			Where("appointments.doctor_id = ?", doctorID)
	}
	if err := query.
		Order("refunds.created_at ASC, refunds.id ASC").
		Limit(limit).
		Offset(offset).
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list manual refunds: %w", err)
	}

	refunds := make([]*models.Refund, 0, len(records))
	for _, record := range records {
		refunds = append(refunds, toRefundModel(record))
	}

	return refunds, nil
}

func toPaymentModel(record paymentRecord) *models.Payment {
	payment := &models.Payment{
		ID:              record.ID,
		AppointmentID:   record.AppointmentID,
		PayerUserID:     record.PayerUserID,
		Amount:          record.Amount,
		Currency:        record.Currency,
		Provider:        record.Provider,
		ProviderOrderID: record.ProviderOrderID,
		Status:          models.PaymentStatus(record.Status),
		CapturedAt:      record.CapturedAt,
		ProofStorageKey: record.ProofStorageKey,
		ClaimedAt:       record.ClaimedAt,
		ReviewedAt:      record.ReviewedAt,
		ReviewNote:      record.ReviewNote,
		CreatedAt:       record.CreatedAt,
		UpdatedAt:       record.UpdatedAt,
	}
	if record.ProviderPaymentID != nil {
		payment.ProviderPaymentID = *record.ProviderPaymentID
	}
	if record.PayerReference != nil {
		payment.PayerReference = *record.PayerReference
	}
	if record.ReviewedByUserID != nil {
		payment.ReviewedByUserID = *record.ReviewedByUserID
	}

	return payment
}

func toRefundModel(record refundRecord) *models.Refund {
	refund := &models.Refund{
		ID:          record.ID,
		PaymentID:   record.PaymentID,
		Amount:      record.Amount,
		Reason:      models.RefundReason(record.Reason),
		Note:        record.Note,
		Status:      models.RefundStatus(record.Status),
		ProcessedAt: record.ProcessedAt,
		CreatedAt:   record.CreatedAt,
		UpdatedAt:   record.UpdatedAt,
	}
	if record.InitiatedByUserID != nil {
		refund.InitiatedByUserID = *record.InitiatedByUserID
	}
	if record.ProviderRefundID != nil {
		refund.ProviderRefundID = *record.ProviderRefundID
	}

	return refund
}
