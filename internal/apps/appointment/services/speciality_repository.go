package services

import (
	"appointment_system/internal/apps/appointment/models"
	"context"
)

type SpecialityRepository interface {
	Create(ctx context.Context, speciality *models.Speciality) (*models.Speciality, error)
	GetBySlugs(ctx context.Context, slugs []string) (map[string]*models.Speciality, error)
	List(ctx context.Context) ([]*models.Speciality, error)
}
