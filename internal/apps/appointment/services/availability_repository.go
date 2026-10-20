package services

import (
	"appointment_system/internal/apps/appointment/models"
	"context"
	"time"
)

type AvailabilityRepository interface {
	ReplaceRules(ctx context.Context, doctorID uint64, rules []models.AvailabilityRule) ([]*models.AvailabilityRule, error)
	ListRules(ctx context.Context, doctorID uint64) ([]*models.AvailabilityRule, error)
	ListRulesByDay(ctx context.Context, doctorID uint64, dayOfWeek time.Weekday) ([]*models.AvailabilityRule, error)
	CreateOverride(ctx context.Context, override *models.AvailabilityOverride) (*models.AvailabilityOverride, error)
	ListOverrides(ctx context.Context, doctorID uint64, fromDate time.Time, toDate time.Time) ([]*models.AvailabilityOverride, error)
}
