package repositories

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type CallJoinAttemptRepository struct {
	db *gorm.DB
}

func NewCallJoinAttemptRepository(db *gorm.DB) (*CallJoinAttemptRepository, error) {
	if db == nil {
		return nil, apperrors.ErrNilDB
	}
	return &CallJoinAttemptRepository{db: db}, nil
}

type callJoinAttemptRecord struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement;column:id"`
	ClientIP  string    `gorm:"column:client_ip"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (callJoinAttemptRecord) TableName() string {
	return "call_join_attempts"
}

func (repo *CallJoinAttemptRepository) Create(ctx context.Context, clientIP string, at time.Time) error {
	record := callJoinAttemptRecord{
		ClientIP:  clientIP,
		CreatedAt: at.UTC(),
	}

	if err := dbFromContext(ctx, repo.db).Create(&record).Error; err != nil {
		return fmt.Errorf("create call join attempt record: %w", err)
	}

	return nil
}

func (repo *CallJoinAttemptRepository) CountSince(ctx context.Context, clientIP string, since time.Time) (int64, error) {
	var count int64

	if err := dbFromContext(ctx, repo.db).
		Model(&callJoinAttemptRecord{}).
		Where("client_ip = ? AND created_at > ?", clientIP, since.UTC()).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count call join attempts: %w", err)
	}

	return count, nil
}

func (repo *CallJoinAttemptRepository) DeleteBefore(ctx context.Context, clientIP string, before time.Time) error {
	if err := dbFromContext(ctx, repo.db).
		Where("client_ip = ? AND created_at <= ?", clientIP, before.UTC()).
		Delete(&callJoinAttemptRecord{}).Error; err != nil {
		return fmt.Errorf("delete call join attempts: %w", err)
	}

	return nil
}
