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

type PatientPassRepository struct {
	db *gorm.DB
}

func NewPatientPassRepository(db *gorm.DB) (*PatientPassRepository, error) {
	if db == nil {
		return nil, apperrors.ErrNilDB
	}
	return &PatientPassRepository{db: db}, nil
}

type patientPassRecord struct {
	ID              uint64    `gorm:"primaryKey;autoIncrement;column:id"`
	PatientID       uint64    `gorm:"column:patient_id"`
	CreatedByUserID uint64    `gorm:"column:created_by_user_id"`
	TokenHash       string    `gorm:"column:token_hash"`
	CodeHash        string    `gorm:"column:code_hash"`
	ExpiresAt       time.Time `gorm:"column:expires_at"`
	CreatedAt       time.Time `gorm:"column:created_at"`
}

func (patientPassRecord) TableName() string {
	return "patient_passes"
}

// Replace stores the patient's new pass in place of the old one, so the old
// QR and code stop working at once. A token or code clashing with another
// patient's pass is ErrPatientPassCodeExists.
func (repo *PatientPassRepository) Replace(ctx context.Context, pass *models.PatientPass) (*models.PatientPass, error) {
	if pass == nil {
		return nil, apperrors.ErrInvalidDeskInput
	}

	record := patientPassRecord{
		PatientID:       pass.PatientID,
		CreatedByUserID: pass.CreatedByUserID,
		TokenHash:       pass.TokenHash,
		CodeHash:        pass.CodeHash,
		ExpiresAt:       pass.ExpiresAt.UTC(),
		CreatedAt:       time.Now().UTC(),
	}

	if err := dbFromContext(ctx, repo.db).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "patient_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"created_by_user_id", "token_hash", "code_hash", "expires_at", "created_at"}),
		}).
		Create(&record).Error; err != nil {
		if patientPassTokenHashUnique.ViolatedBy(err) || patientPassCodeHashUnique.ViolatedBy(err) {
			return nil, apperrors.ErrPatientPassCodeExists
		}
		return nil, fmt.Errorf("replace patient pass record: %w", err)
	}

	return toPatientPassModel(record), nil
}

func (repo *PatientPassRepository) GetByTokenHash(ctx context.Context, tokenHash string) (*models.PatientPass, error) {
	return repo.getBy(ctx, "token_hash = ?", tokenHash)
}

func (repo *PatientPassRepository) GetByCodeHash(ctx context.Context, codeHash string) (*models.PatientPass, error) {
	return repo.getBy(ctx, "code_hash = ?", codeHash)
}

func (repo *PatientPassRepository) getBy(ctx context.Context, condition string, value string) (*models.PatientPass, error) {
	var record patientPassRecord

	if err := dbFromContext(ctx, repo.db).Where(condition, value).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrPatientPassInvalid
		}
		return nil, fmt.Errorf("get patient pass: %w", err)
	}

	return toPatientPassModel(record), nil
}

func toPatientPassModel(record patientPassRecord) *models.PatientPass {
	return &models.PatientPass{
		ID:              record.ID,
		PatientID:       record.PatientID,
		CreatedByUserID: record.CreatedByUserID,
		TokenHash:       record.TokenHash,
		CodeHash:        record.CodeHash,
		ExpiresAt:       record.ExpiresAt,
		CreatedAt:       record.CreatedAt,
	}
}
