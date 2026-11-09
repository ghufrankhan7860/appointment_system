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

	if err := dbFromContext(ctx, repo.db).Create(&record).Error; err != nil {
		if emailUnique.ViolatedBy(err) {
			return nil, apperrors.ErrEmailAlreadyExists
		}
		if mobileUnique.ViolatedBy(err) {
			return nil, apperrors.ErrMobileAlreadyExists
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

// FindByEmail finds the user signing in with an email. Emails are stored
// lowercased, so a plain equality uses the unique index.
func (repo *UserRepository) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	return repo.findForLogin(ctx, "email = ?", strings.ToLower(strings.TrimSpace(email)))
}

// FindByMobile finds the user signing in with a mobile number.
func (repo *UserRepository) FindByMobile(ctx context.Context, mobileNo string) (*models.User, error) {
	return repo.findForLogin(ctx, "mobile_no = ?", strings.TrimSpace(mobileNo))
}

// findForLogin looks a user up by one login column; an unknown one reads as
// invalid credentials.
func (repo *UserRepository) findForLogin(ctx context.Context, condition string, value string) (*models.User, error) {
	var record userRecord

	if err := dbFromContext(ctx, repo.db).
		Where(condition, value).
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

	if err := dbFromContext(ctx, repo.db).
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

// GetByIDs leaves out password hashes; it serves listings, never sign-in.
func (repo *UserRepository) GetByIDs(ctx context.Context, ids []uint64) (map[uint64]*models.User, error) {
	result := make(map[uint64]*models.User, len(ids))
	if len(ids) == 0 {
		return result, nil
	}

	var records []userRecord
	if err := dbFromContext(ctx, repo.db).
		Where("id IN ?", ids).
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("get users by ids: %w", err)
	}

	for _, record := range records {
		result[record.ID] = &models.User{
			ID:        record.ID,
			Name:      record.Name,
			Email:     record.Email,
			MobileNo:  record.MobileNo,
			Role:      models.Role(record.Role),
			CreatedAt: record.CreatedAt,
			UpdatedAt: record.UpdatedAt,
		}
	}

	return result, nil
}

func (repo *UserRepository) ExistsByRole(ctx context.Context, role models.Role) (bool, error) {
	var count int64

	if err := dbFromContext(ctx, repo.db).
		Model(&userRecord{}).
		Where("role = ?", string(role)).
		Limit(1).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("count users by role: %w", err)
	}

	return count > 0, nil
}
