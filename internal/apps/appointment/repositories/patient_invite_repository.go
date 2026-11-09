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

type PatientInviteRepository struct {
	db *gorm.DB
}

func NewPatientInviteRepository(db *gorm.DB) (*PatientInviteRepository, error) {
	if db == nil {
		return nil, apperrors.ErrNilDB
	}
	return &PatientInviteRepository{db: db}, nil
}

type patientInviteRecord struct {
	ID                  uint64     `gorm:"primaryKey;autoIncrement;column:id"`
	PatientID           uint64     `gorm:"column:patient_id"`
	CreatedByUserID     uint64     `gorm:"column:created_by_user_id"`
	CodeHash            string     `gorm:"column:code_hash"`
	PatientRelationship string     `gorm:"column:patient_relationship"`
	AccessLevel         string     `gorm:"column:access_level"`
	Status              string     `gorm:"column:status"`
	ExpiresAt           time.Time  `gorm:"column:expires_at"`
	RedeemedByUserID    *uint64    `gorm:"column:redeemed_by_user_id"`
	RedeemedAt          *time.Time `gorm:"column:redeemed_at"`
	CreatedAt           time.Time  `gorm:"column:created_at"`
	UpdatedAt           time.Time  `gorm:"column:updated_at"`
}

func (patientInviteRecord) TableName() string {
	return "patient_invites"
}

func (repo *PatientInviteRepository) Create(ctx context.Context, invite *models.PatientInvite) (*models.PatientInvite, error) {
	if invite == nil {
		return nil, apperrors.ErrNilPatientInviteRequest
	}

	now := time.Now().UTC()
	record := patientInviteRecord{
		PatientID:           invite.PatientID,
		CreatedByUserID:     invite.CreatedByUserID,
		CodeHash:            invite.CodeHash,
		PatientRelationship: string(invite.PatientRelationship),
		AccessLevel:         string(invite.AccessLevel),
		Status:              string(invite.Status),
		ExpiresAt:           invite.ExpiresAt.UTC(),
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	if err := dbFromContext(ctx, repo.db).Create(&record).Error; err != nil {
		if patientInviteCodeHashUnique.ViolatedBy(err) {
			return nil, apperrors.ErrPatientInviteCodeExists
		}
		return nil, fmt.Errorf("create patient invite record: %w", err)
	}

	return toPatientInviteModel(record), nil
}

func (repo *PatientInviteRepository) GetByCodeHashForUpdate(ctx context.Context, codeHash string) (*models.PatientInvite, error) {
	var record patientInviteRecord

	if err := dbFromContext(ctx, repo.db).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("code_hash = ?", codeHash).
		First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrPatientInviteNotFound
		}
		return nil, fmt.Errorf("get patient invite by code hash: %w", err)
	}

	return toPatientInviteModel(record), nil
}

func (repo *PatientInviteRepository) ListPendingByPatient(ctx context.Context, patientID uint64, now time.Time) ([]*models.PatientInvite, error) {
	var records []patientInviteRecord

	if err := dbFromContext(ctx, repo.db).
		Where("patient_id = ? AND status = ? AND expires_at > ?", patientID, string(models.PatientInviteStatusPending), now.UTC()).
		Order("created_at DESC, id DESC").
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list pending patient invites: %w", err)
	}

	result := make([]*models.PatientInvite, 0, len(records))
	for _, record := range records {
		result = append(result, toPatientInviteModel(record))
	}
	return result, nil
}

func (repo *PatientInviteRepository) CountPendingByPatient(ctx context.Context, patientID uint64, now time.Time) (int64, error) {
	var count int64

	if err := dbFromContext(ctx, repo.db).
		Model(&patientInviteRecord{}).
		Where("patient_id = ? AND status = ? AND expires_at > ?", patientID, string(models.PatientInviteStatusPending), now.UTC()).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count pending patient invites: %w", err)
	}

	return count, nil
}

func (repo *PatientInviteRepository) MarkRedeemed(ctx context.Context, id uint64, redeemedByUserID uint64, redeemedAt time.Time) error {
	return repo.updatePending(ctx, "id = ?", []any{id}, map[string]interface{}{
		"status":              string(models.PatientInviteStatusRedeemed),
		"redeemed_by_user_id": redeemedByUserID,
		"redeemed_at":         redeemedAt.UTC(),
	})
}

func (repo *PatientInviteRepository) Revoke(ctx context.Context, patientID uint64, id uint64) error {
	return repo.updatePending(ctx, "id = ? AND patient_id = ?", []any{id, patientID}, map[string]interface{}{
		"status": string(models.PatientInviteStatusRevoked),
	})
}

func (repo *PatientInviteRepository) MarkExpired(ctx context.Context, id uint64) error {
	return repo.updatePending(ctx, "id = ?", []any{id}, map[string]interface{}{
		"status": string(models.PatientInviteStatusExpired),
	})
}

func (repo *PatientInviteRepository) updatePending(ctx context.Context, condition string, args []any, updates map[string]interface{}) error {
	updates["updated_at"] = time.Now().UTC()

	result := dbFromContext(ctx, repo.db).
		Model(&patientInviteRecord{}).
		Where(condition, args...).
		Where("status = ?", string(models.PatientInviteStatusPending)).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("update patient invite record: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrPatientInviteNotFound
	}

	return nil
}

func toPatientInviteModel(record patientInviteRecord) *models.PatientInvite {
	return &models.PatientInvite{
		ID:                  record.ID,
		PatientID:           record.PatientID,
		CreatedByUserID:     record.CreatedByUserID,
		CodeHash:            record.CodeHash,
		PatientRelationship: models.Relationship(record.PatientRelationship),
		AccessLevel:         models.AccessLevel(record.AccessLevel),
		Status:              models.PatientInviteStatus(record.Status),
		ExpiresAt:           record.ExpiresAt,
		RedeemedByUserID:    record.RedeemedByUserID,
		RedeemedAt:          record.RedeemedAt,
		CreatedAt:           record.CreatedAt,
		UpdatedAt:           record.UpdatedAt,
	}
}
