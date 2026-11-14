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

type StaffInviteRepository struct {
	db *gorm.DB
}

func NewStaffInviteRepository(db *gorm.DB) (*StaffInviteRepository, error) {
	if db == nil {
		return nil, apperrors.ErrNilDB
	}
	return &StaffInviteRepository{db: db}, nil
}

type staffInviteRecord struct {
	ID               uint64     `gorm:"primaryKey;autoIncrement;column:id"`
	DoctorID         uint64     `gorm:"column:doctor_id"`
	CreatedByUserID  uint64     `gorm:"column:created_by_user_id"`
	CodeHash         string     `gorm:"column:code_hash"`
	Status           string     `gorm:"column:status"`
	ExpiresAt        time.Time  `gorm:"column:expires_at"`
	RedeemedByUserID *uint64    `gorm:"column:redeemed_by_user_id"`
	RedeemedAt       *time.Time `gorm:"column:redeemed_at"`
	CreatedAt        time.Time  `gorm:"column:created_at"`
	UpdatedAt        time.Time  `gorm:"column:updated_at"`
}

func (staffInviteRecord) TableName() string {
	return "doctor_staff_invites"
}

func (repo *StaffInviteRepository) Create(ctx context.Context, invite *models.StaffInvite) (*models.StaffInvite, error) {
	if invite == nil {
		return nil, apperrors.ErrInvalidStaffInput
	}

	now := time.Now().UTC()
	record := staffInviteRecord{
		DoctorID:        invite.DoctorID,
		CreatedByUserID: invite.CreatedByUserID,
		CodeHash:        invite.CodeHash,
		Status:          string(invite.Status),
		ExpiresAt:       invite.ExpiresAt.UTC(),
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := dbFromContext(ctx, repo.db).Create(&record).Error; err != nil {
		if staffInviteCodeHashUnique.ViolatedBy(err) {
			return nil, apperrors.ErrStaffInviteCodeExists
		}
		return nil, fmt.Errorf("create staff invite record: %w", err)
	}

	return toStaffInviteModel(record), nil
}

func (repo *StaffInviteRepository) GetByCodeHashForUpdate(ctx context.Context, codeHash string) (*models.StaffInvite, error) {
	var record staffInviteRecord

	if err := dbFromContext(ctx, repo.db).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("code_hash = ?", codeHash).
		First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrStaffInviteNotFound
		}
		return nil, fmt.Errorf("get staff invite by code hash: %w", err)
	}

	return toStaffInviteModel(record), nil
}

func (repo *StaffInviteRepository) ListPendingByDoctor(ctx context.Context, doctorID uint64, now time.Time) ([]*models.StaffInvite, error) {
	var records []staffInviteRecord

	if err := dbFromContext(ctx, repo.db).
		Where("doctor_id = ? AND status = ? AND expires_at > ?", doctorID, string(models.StaffInviteStatusPending), now.UTC()).
		Order("created_at DESC, id DESC").
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list pending staff invites: %w", err)
	}

	result := make([]*models.StaffInvite, 0, len(records))
	for _, record := range records {
		result = append(result, toStaffInviteModel(record))
	}
	return result, nil
}

func (repo *StaffInviteRepository) CountPendingByDoctor(ctx context.Context, doctorID uint64, now time.Time) (int64, error) {
	var count int64

	if err := dbFromContext(ctx, repo.db).
		Model(&staffInviteRecord{}).
		Where("doctor_id = ? AND status = ? AND expires_at > ?", doctorID, string(models.StaffInviteStatusPending), now.UTC()).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count pending staff invites: %w", err)
	}

	return count, nil
}

func (repo *StaffInviteRepository) MarkRedeemed(ctx context.Context, id uint64, redeemedByUserID uint64, redeemedAt time.Time) error {
	return repo.updatePending(ctx, "id = ?", []any{id}, map[string]interface{}{
		"status":              string(models.StaffInviteStatusRedeemed),
		"redeemed_by_user_id": redeemedByUserID,
		"redeemed_at":         redeemedAt.UTC(),
	})
}

func (repo *StaffInviteRepository) Revoke(ctx context.Context, doctorID uint64, id uint64) error {
	return repo.updatePending(ctx, "id = ? AND doctor_id = ?", []any{id, doctorID}, map[string]interface{}{
		"status": string(models.StaffInviteStatusRevoked),
	})
}

func (repo *StaffInviteRepository) MarkExpired(ctx context.Context, id uint64) error {
	return repo.updatePending(ctx, "id = ?", []any{id}, map[string]interface{}{
		"status": string(models.StaffInviteStatusExpired),
	})
}

func (repo *StaffInviteRepository) updatePending(ctx context.Context, condition string, args []any, updates map[string]interface{}) error {
	updates["updated_at"] = time.Now().UTC()

	result := dbFromContext(ctx, repo.db).
		Model(&staffInviteRecord{}).
		Where(condition, args...).
		Where("status = ?", string(models.StaffInviteStatusPending)).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("update staff invite record: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrStaffInviteNotFound
	}

	return nil
}

func toStaffInviteModel(record staffInviteRecord) *models.StaffInvite {
	return &models.StaffInvite{
		ID:               record.ID,
		DoctorID:         record.DoctorID,
		CreatedByUserID:  record.CreatedByUserID,
		CodeHash:         record.CodeHash,
		Status:           models.StaffInviteStatus(record.Status),
		ExpiresAt:        record.ExpiresAt,
		RedeemedByUserID: record.RedeemedByUserID,
		RedeemedAt:       record.RedeemedAt,
		CreatedAt:        record.CreatedAt,
		UpdatedAt:        record.UpdatedAt,
	}
}
