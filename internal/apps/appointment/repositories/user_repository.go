package repositories

import (
	"appointment_system/internal/apps/appointment/apperrors"
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
		return nil, apperrors.ErrNilDB
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
		return nil, apperrors.ErrNilRegisterRequest
	}

	now := time.Now().UTC()

	record := userRecord{
		Name:         user.Name,
		Email:        strings.ToLower(strings.TrimSpace(user.Email)),
		PasswordHash: user.PasswordHash,
		MobileNo:     strings.TrimSpace(user.MobileNo),
		Role:         string(user.Role),
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := repo.db.WithContext(ctx).Create(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, apperrors.ErrUserAlreadyExists
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

func (repo *UserRepository) FindByLogin(ctx context.Context, identifier string) (*models.User, error) {
	var record userRecord

	emailIdentifier := strings.ToLower(strings.TrimSpace(identifier))
	mobileIdentifier := strings.TrimSpace(identifier)

	if err := repo.db.WithContext(ctx).
		Where("LOWER(email) = ? OR mobile_no = ?", emailIdentifier, mobileIdentifier).
		First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrInvalidCredentials
		}

		return nil, fmt.Errorf("find user by login: %w", err)
	}

	return &models.User{
		ID:           record.ID,
		Name:         record.Name,
		Email:        record.Email,
		PasswordHash: record.PasswordHash,
		MobileNo:     record.MobileNo,
		Role:         models.Role(record.Role),
		CreatedAt:    record.CreatedAt,
		UpdatedAt:    record.UpdatedAt,
	}, nil
}

func (repo *UserRepository) GetByID(ctx context.Context, id uint64) (*models.User, error) {
	var record userRecord

	if err := repo.db.WithContext(ctx).
		Where("id = ?", id).
		First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrUserNotFound
		}

		return nil, fmt.Errorf("get user by id: %w", err)
	}

	return &models.User{
		ID:           record.ID,
		Name:         record.Name,
		Email:        record.Email,
		PasswordHash: record.PasswordHash,
		MobileNo:     record.MobileNo,
		Role:         models.Role(record.Role),
		CreatedAt:    record.CreatedAt,
		UpdatedAt:    record.UpdatedAt,
	}, nil
}
