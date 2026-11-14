package repositories

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type StaffInviteAttemptRepository struct {
	db *gorm.DB
}

func NewStaffInviteAttemptRepository(db *gorm.DB) (*StaffInviteAttemptRepository, error) {
	if db == nil {
		return nil, apperrors.ErrNilDB
	}
	return &StaffInviteAttemptRepository{db: db}, nil
}

type staffInviteAttemptRecord struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement;column:id"`
	ClientIP  string    `gorm:"column:client_ip"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (staffInviteAttemptRecord) TableName() string {
	return "staff_invite_attempts"
}

func (repo *StaffInviteAttemptRepository) Create(ctx context.Context, clientIP string, at time.Time) error {
	record := staffInviteAttemptRecord{
		ClientIP:  clientIP,
		CreatedAt: at.UTC(),
	}

	if err := dbFromContext(ctx, repo.db).Create(&record).Error; err != nil {
		return fmt.Errorf("create staff invite attempt record: %w", err)
	}

	return nil
}

func (repo *StaffInviteAttemptRepository) CountSince(ctx context.Context, clientIP string, since time.Time) (int64, error) {
	var count int64

	if err := dbFromContext(ctx, repo.db).
		Model(&staffInviteAttemptRecord{}).
		Where("client_ip = ? AND created_at > ?", clientIP, since.UTC()).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count staff invite attempts: %w", err)
	}

	return count, nil
}

func (repo *StaffInviteAttemptRepository) DeleteBefore(ctx context.Context, clientIP string, before time.Time) error {
	if err := dbFromContext(ctx, repo.db).
		Where("client_ip = ? AND created_at <= ?", clientIP, before.UTC()).
		Delete(&staffInviteAttemptRecord{}).Error; err != nil {
		return fmt.Errorf("delete staff invite attempts: %w", err)
	}

	return nil
}
