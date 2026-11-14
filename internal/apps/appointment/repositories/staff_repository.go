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

type StaffRepository struct {
	db *gorm.DB
}

func NewStaffRepository(db *gorm.DB) (*StaffRepository, error) {
	if db == nil {
		return nil, apperrors.ErrNilDB
	}
	return &StaffRepository{db: db}, nil
}

type doctorStaffRecord struct {
	ID              uint64     `gorm:"primaryKey;autoIncrement;column:id"`
	DoctorID        uint64     `gorm:"column:doctor_id"`
	UserID          uint64     `gorm:"column:user_id"`
	Status          string     `gorm:"column:status"`
	RemovedAt       *time.Time `gorm:"column:removed_at"`
	RemovedByUserID *uint64    `gorm:"column:removed_by_user_id"`
	CreatedAt       time.Time  `gorm:"column:created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at"`
}

func (doctorStaffRecord) TableName() string {
	return "doctor_staff"
}

type staffMemberRecord struct {
	Staff    doctorStaffRecord `gorm:"embedded"`
	Name     string            `gorm:"column:name"`
	Email    string            `gorm:"column:email"`
	MobileNo string            `gorm:"column:mobile_no"`
}

// Create adds an active staff link. A user already working for a doctor is
// ErrStaffAlreadyLinked.
func (repo *StaffRepository) Create(ctx context.Context, staff *models.DoctorStaff) (*models.DoctorStaff, error) {
	if staff == nil {
		return nil, apperrors.ErrInvalidStaffInput
	}

	now := time.Now().UTC()
	record := doctorStaffRecord{
		DoctorID:  staff.DoctorID,
		UserID:    staff.UserID,
		Status:    string(models.StaffStatusActive),
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := dbFromContext(ctx, repo.db).Create(&record).Error; err != nil {
		if doctorStaffActiveUnique.ViolatedBy(err) {
			return nil, apperrors.ErrStaffAlreadyLinked
		}
		return nil, fmt.Errorf("create doctor staff record: %w", err)
	}

	return toDoctorStaffModel(record), nil
}

// GetActiveByUser returns the doctor a staff account works for now.
func (repo *StaffRepository) GetActiveByUser(ctx context.Context, userID uint64) (*models.DoctorStaff, error) {
	var record doctorStaffRecord

	if err := dbFromContext(ctx, repo.db).
		Where("user_id = ? AND status = ?", userID, string(models.StaffStatusActive)).
		First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrStaffNotFound
		}
		return nil, fmt.Errorf("get active doctor staff: %w", err)
	}

	return toDoctorStaffModel(record), nil
}

// ListActiveByDoctor returns the doctor's current staff, oldest first.
func (repo *StaffRepository) ListActiveByDoctor(ctx context.Context, doctorID uint64) ([]*models.StaffMember, error) {
	var records []staffMemberRecord

	if err := dbFromContext(ctx, repo.db).
		Table("doctor_staff").
		Select("doctor_staff.*, users.name, users.email, users.mobile_no").
		Joins("JOIN users ON users.id = doctor_staff.user_id").
		Where("doctor_staff.doctor_id = ? AND doctor_staff.status = ?", doctorID, string(models.StaffStatusActive)).
		Order("doctor_staff.created_at ASC, doctor_staff.id ASC").
		Scan(&records).Error; err != nil {
		return nil, fmt.Errorf("list doctor staff: %w", err)
	}

	result := make([]*models.StaffMember, 0, len(records))
	for _, record := range records {
		result = append(result, &models.StaffMember{
			DoctorStaff: *toDoctorStaffModel(record.Staff),
			Name:        record.Name,
			Email:       record.Email,
			MobileNo:    record.MobileNo,
		})
	}
	return result, nil
}

// Remove ends a staff member's link to the doctor.
func (repo *StaffRepository) Remove(ctx context.Context, doctorID uint64, userID uint64, removedByUserID uint64, removedAt time.Time) error {
	result := dbFromContext(ctx, repo.db).
		Model(&doctorStaffRecord{}).
		Where("doctor_id = ? AND user_id = ? AND status = ?", doctorID, userID, string(models.StaffStatusActive)).
		Updates(map[string]interface{}{
			"status":             string(models.StaffStatusRemoved),
			"removed_at":         removedAt.UTC(),
			"removed_by_user_id": removedByUserID,
			"updated_at":         time.Now().UTC(),
		})
	if result.Error != nil {
		return fmt.Errorf("remove doctor staff: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrStaffNotFound
	}

	return nil
}

func toDoctorStaffModel(record doctorStaffRecord) *models.DoctorStaff {
	return &models.DoctorStaff{
		ID:              record.ID,
		DoctorID:        record.DoctorID,
		UserID:          record.UserID,
		Status:          models.StaffStatus(record.Status),
		RemovedAt:       record.RemovedAt,
		RemovedByUserID: record.RemovedByUserID,
		CreatedAt:       record.CreatedAt,
		UpdatedAt:       record.UpdatedAt,
	}
}
