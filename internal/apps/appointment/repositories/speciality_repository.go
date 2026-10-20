package repositories

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type SpecialityRepository struct {
	db *gorm.DB
}

func NewSpecialityRepository(db *gorm.DB) (*SpecialityRepository, error) {
	if db == nil {
		return nil, apperrors.ErrNilDB
	}
	return &SpecialityRepository{db: db}, nil
}

type specialityRecord struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement;column:id"`
	Slug      string    `gorm:"column:slug"`
	Name      string    `gorm:"column:name"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (specialityRecord) TableName() string {
	return "specialities"
}

func (repo *SpecialityRepository) Create(ctx context.Context, speciality *models.Speciality) (*models.Speciality, error) {
	if speciality == nil {
		return nil, apperrors.ErrNilSpecialityRequest
	}

	record := specialityRecord{
		Slug:      speciality.Slug,
		Name:      speciality.Name,
		CreatedAt: time.Now().UTC(),
	}

	if err := dbFromContext(ctx, repo.db).Create(&record).Error; err != nil {
		if specialitySlugUnique.ViolatedBy(err) {
			return nil, apperrors.ErrSpecialityAlreadyExists
		}
		return nil, fmt.Errorf("create speciality record: %w", err)
	}

	return toSpecialityModel(record), nil
}

// GetBySlugs returns the specialities found for the given slugs, keyed by slug.
// Missing slugs are simply absent from the map.
func (repo *SpecialityRepository) GetBySlugs(ctx context.Context, slugs []string) (map[string]*models.Speciality, error) {
	result := make(map[string]*models.Speciality, len(slugs))
	if len(slugs) == 0 {
		return result, nil
	}

	var records []specialityRecord
	if err := dbFromContext(ctx, repo.db).
		Where("slug IN ?", slugs).
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("get specialities by slugs: %w", err)
	}

	for _, record := range records {
		result[record.Slug] = toSpecialityModel(record)
	}
	return result, nil
}

func (repo *SpecialityRepository) List(ctx context.Context) ([]*models.Speciality, error) {
	var records []specialityRecord

	if err := dbFromContext(ctx, repo.db).
		Order("name ASC").
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list specialities: %w", err)
	}

	result := make([]*models.Speciality, 0, len(records))
	for _, record := range records {
		result = append(result, toSpecialityModel(record))
	}
	return result, nil
}

func toSpecialityModel(record specialityRecord) *models.Speciality {
	return &models.Speciality{
		ID:        record.ID,
		Slug:      record.Slug,
		Name:      record.Name,
		CreatedAt: record.CreatedAt,
	}
}
