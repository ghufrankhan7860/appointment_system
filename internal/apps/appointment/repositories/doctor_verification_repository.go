package repositories

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type DoctorVerificationRepository struct {
	db *gorm.DB
}

func NewDoctorVerificationRepository(db *gorm.DB) (*DoctorVerificationRepository, error) {
	if db == nil {
		return nil, apperrors.ErrNilDB
	}
	return &DoctorVerificationRepository{db: db}, nil
}

type doctorVerificationRecord struct {
	ID               uint64    `gorm:"primaryKey;autoIncrement;column:id"`
	DoctorID         uint64    `gorm:"column:doctor_id"`
	ReviewedByUserID uint64    `gorm:"column:reviewed_by_user_id"`
	FromStatus       string    `gorm:"column:from_status"`
	ToStatus         string    `gorm:"column:to_status"`
	Reason           string    `gorm:"column:reason"`
	CreatedAt        time.Time `gorm:"column:created_at"`
}

func (doctorVerificationRecord) TableName() string {
	return "doctor_verifications"
}

func (repo *DoctorVerificationRepository) Create(ctx context.Context, verification *models.DoctorVerification) error {
	if verification == nil {
		return apperrors.ErrNilDoctorVerificationRequest
	}

	record := doctorVerificationRecord{
		DoctorID:         verification.DoctorID,
		ReviewedByUserID: verification.ReviewedByUserID,
		FromStatus:       string(verification.FromStatus),
		ToStatus:         string(verification.ToStatus),
		Reason:           verification.Reason,
		CreatedAt:        time.Now().UTC(),
	}

	if err := dbFromContext(ctx, repo.db).Create(&record).Error; err != nil {
		return fmt.Errorf("create doctor verification record: %w", err)
	}

	return nil
}

func (repo *DoctorVerificationRepository) ListByDoctor(ctx context.Context, doctorID uint64) ([]*models.DoctorVerification, error) {
	var records []doctorVerificationRecord

	if err := dbFromContext(ctx, repo.db).
		Where("doctor_id = ?", doctorID).
		Order("created_at DESC, id DESC").
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list doctor verifications: %w", err)
	}

	result := make([]*models.DoctorVerification, 0, len(records))
	for _, record := range records {
		result = append(result, &models.DoctorVerification{
			ID:               record.ID,
			DoctorID:         record.DoctorID,
			ReviewedByUserID: record.ReviewedByUserID,
			FromStatus:       models.VerificationStatus(record.FromStatus),
			ToStatus:         models.VerificationStatus(record.ToStatus),
			Reason:           record.Reason,
			CreatedAt:        record.CreatedAt,
		})
	}
	return result, nil
}
