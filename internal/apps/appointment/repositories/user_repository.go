package repositories

import (
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) (*UserRepository, error) {
	if db == nil {
		return nil, constant.ErrNilDB
	}
	return &UserRepository{db: db}, nil
}

type userRecord struct {
	ID           uint64 `gorm:"primaryKey"`
	Name         string
	Email        string `gorm:"uniqueIndex"`
	PasswordHash []byte
	MobileNo     string
	Role         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (userRecord) TableName() string {
	return "users"
}

func (repo *UserRepository) Create(ctx context.Context, user *models.User) (*models.User, error) {
	if user == nil {
		return nil, constant.ErrNilRegisterRequest
	}

	record := userRecord{
		Name:         user.Name,
		Email:        strings.ToLower(strings.TrimSpace(user.Email)),
		PasswordHash: user.PasswordHash,
		MobileNo:     user.MobileNo,
		Role:         string(user.Role),
	}
	if err := repo.db.WithContext(ctx).Create(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, constant.ErrUserAlreadyExists
		}
		return nil, fmt.Errorf("create user record: %w", err)
	}
	return &models.User{
		ID:        record.ID,
		Name:      record.Name,
		Email:     record.Email,
		MobileNo:  record.MobileNo,
		Role:      models.Role(record.Role),
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
	}, nil
}
