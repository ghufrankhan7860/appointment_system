package repositories

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/utils"
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type AvailabilityRepository struct {
	db *gorm.DB
}

func NewAvailabilityRepository(db *gorm.DB) (*AvailabilityRepository, error) {
	if db == nil {
		return nil, apperrors.ErrNilDB
	}
	return &AvailabilityRepository{db: db}, nil
}

type availabilityRuleRecord struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement;column:id"`
	DoctorID    uint64    `gorm:"column:doctor_id"`
	DayOfWeek   int       `gorm:"column:day_of_week"`
	StartMinute int       `gorm:"column:start_minute"`
	EndMinute   int       `gorm:"column:end_minute"`
	Mode        string    `gorm:"column:mode"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (availabilityRuleRecord) TableName() string {
	return "doctor_availability"
}

type availabilityOverrideRecord struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement;column:id"`
	DoctorID     uint64    `gorm:"column:doctor_id"`
	OverrideDate time.Time `gorm:"column:override_date"`
	StartMinute  *int      `gorm:"column:start_minute"`
	EndMinute    *int      `gorm:"column:end_minute"`
	IsAvailable  bool      `gorm:"column:is_available"`
	Mode         string    `gorm:"column:mode"`
	Reason       string    `gorm:"column:reason"`
	CreatedAt    time.Time `gorm:"column:created_at"`
}

func (availabilityOverrideRecord) TableName() string {
	return "doctor_availability_overrides"
}

// ReplaceRules deletes the doctor's weekly rules and inserts the new set.
// Callers run it inside a transaction so a failed insert keeps the old rules.
func (repo *AvailabilityRepository) ReplaceRules(
	ctx context.Context,
	doctorID uint64,
	rules []models.AvailabilityRule,
) ([]*models.AvailabilityRule, error) {
	db := dbFromContext(ctx, repo.db)

	if err := db.Where("doctor_id = ?", doctorID).Delete(&availabilityRuleRecord{}).Error; err != nil {
		return nil, fmt.Errorf("delete availability rules: %w", err)
	}

	result := make([]*models.AvailabilityRule, 0, len(rules))
	if len(rules) == 0 {
		return result, nil
	}

	now := time.Now().UTC()
	records := make([]availabilityRuleRecord, 0, len(rules))
	for _, rule := range rules {
		records = append(records, availabilityRuleRecord{
			DoctorID:    doctorID,
			DayOfWeek:   int(rule.DayOfWeek),
			StartMinute: rule.StartMinute,
			EndMinute:   rule.EndMinute,
			Mode:        string(rule.Mode),
			CreatedAt:   now,
			UpdatedAt:   now,
		})
	}

	if err := db.Create(&records).Error; err != nil {
		return nil, fmt.Errorf("create availability rules: %w", err)
	}

	for _, record := range records {
		result = append(result, toAvailabilityRuleModel(record))
	}
	return result, nil
}

func (repo *AvailabilityRepository) ListRules(ctx context.Context, doctorID uint64) ([]*models.AvailabilityRule, error) {
	var records []availabilityRuleRecord

	if err := dbFromContext(ctx, repo.db).
		Where("doctor_id = ?", doctorID).
		Order("day_of_week ASC, start_minute ASC").
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list availability rules: %w", err)
	}

	result := make([]*models.AvailabilityRule, 0, len(records))
	for _, record := range records {
		result = append(result, toAvailabilityRuleModel(record))
	}
	return result, nil
}

func (repo *AvailabilityRepository) ListRulesByDay(
	ctx context.Context,
	doctorID uint64,
	dayOfWeek time.Weekday,
) ([]*models.AvailabilityRule, error) {
	var records []availabilityRuleRecord

	if err := dbFromContext(ctx, repo.db).
		Where("doctor_id = ? AND day_of_week = ?", doctorID, int(dayOfWeek)).
		Order("start_minute ASC").
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list availability rules by day: %w", err)
	}

	result := make([]*models.AvailabilityRule, 0, len(records))
	for _, record := range records {
		result = append(result, toAvailabilityRuleModel(record))
	}
	return result, nil
}

func (repo *AvailabilityRepository) CreateOverride(
	ctx context.Context,
	override *models.AvailabilityOverride,
) (*models.AvailabilityOverride, error) {
	if override == nil {
		return nil, apperrors.ErrNilOverrideRequest
	}

	record := availabilityOverrideRecord{
		DoctorID:     override.DoctorID,
		OverrideDate: utils.CivilDate(override.Date),
		StartMinute:  override.StartMinute,
		EndMinute:    override.EndMinute,
		IsAvailable:  override.IsAvailable,
		Mode:         string(override.Mode),
		Reason:       override.Reason,
		CreatedAt:    time.Now().UTC(),
	}

	if err := dbFromContext(ctx, repo.db).Create(&record).Error; err != nil {
		return nil, fmt.Errorf("create availability override: %w", err)
	}

	return toAvailabilityOverrideModel(record), nil
}

// ListOverrides returns the doctor's overrides with dates from fromDate to
// toDate inclusive.
func (repo *AvailabilityRepository) ListOverrides(
	ctx context.Context,
	doctorID uint64,
	fromDate time.Time,
	toDate time.Time,
) ([]*models.AvailabilityOverride, error) {
	var records []availabilityOverrideRecord

	if err := dbFromContext(ctx, repo.db).
		Where("doctor_id = ? AND override_date BETWEEN ? AND ?", doctorID, utils.CivilDate(fromDate), utils.CivilDate(toDate)).
		Order("override_date ASC, id ASC").
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list availability overrides: %w", err)
	}

	result := make([]*models.AvailabilityOverride, 0, len(records))
	for _, record := range records {
		result = append(result, toAvailabilityOverrideModel(record))
	}
	return result, nil
}

func toAvailabilityRuleModel(record availabilityRuleRecord) *models.AvailabilityRule {
	return &models.AvailabilityRule{
		ID:          record.ID,
		DoctorID:    record.DoctorID,
		DayOfWeek:   time.Weekday(record.DayOfWeek),
		StartMinute: record.StartMinute,
		EndMinute:   record.EndMinute,
		Mode:        models.ConsultationMode(record.Mode),
		CreatedAt:   record.CreatedAt,
		UpdatedAt:   record.UpdatedAt,
	}
}

func toAvailabilityOverrideModel(record availabilityOverrideRecord) *models.AvailabilityOverride {
	return &models.AvailabilityOverride{
		ID:          record.ID,
		DoctorID:    record.DoctorID,
		Date:        utils.CivilDate(record.OverrideDate),
		StartMinute: record.StartMinute,
		EndMinute:   record.EndMinute,
		IsAvailable: record.IsAvailable,
		Mode:        models.ConsultationMode(record.Mode),
		Reason:      record.Reason,
		CreatedAt:   record.CreatedAt,
	}
}
