package services

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/utils"
)

// AvailabilityService owns a doctor's weekly working rules, one-off overrides,
// and the bookable slots computed from them. Times are wall-clock times in the
// clinic timezone.
type AvailabilityService struct {
	doctors      DoctorRepository
	availability AvailabilityRepository
	appointments AppointmentRepository
	tx           TransactionManager
	location     *time.Location
	now          func() time.Time
}

func NewAvailabilityService(
	doctors DoctorRepository,
	availability AvailabilityRepository,
	appointments AppointmentRepository,
	tx TransactionManager,
	location *time.Location,
	now func() time.Time,
) (*AvailabilityService, error) {
	if doctors == nil {
		return nil, apperrors.ErrNilDoctorRepository
	}
	if availability == nil {
		return nil, apperrors.ErrNilAvailabilityRepository
	}
	if appointments == nil {
		return nil, apperrors.ErrNilAppointmentRepository
	}
	if tx == nil {
		return nil, apperrors.ErrNilTransactionManager
	}
	if location == nil {
		return nil, apperrors.ErrNilClinicLocation
	}
	if now == nil {
		return nil, apperrors.ErrNilClock
	}

	return &AvailabilityService{
		doctors:      doctors,
		availability: availability,
		appointments: appointments,
		tx:           tx,
		location:     location,
		now:          now,
	}, nil
}

// Get returns the acting doctor's weekly rules and the overrides from today on.
func (avs *AvailabilityService) Get(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
) ([]*models.AvailabilityRule, []*models.AvailabilityOverride, error) {
	doctor, err := avs.ownDoctor(ctx, actingUserID, actingRole)
	if err != nil {
		return nil, nil, err
	}

	rules, err := avs.availability.ListRules(ctx, doctor.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("list availability rules: %w", err)
	}

	today := avs.today()
	overrides, err := avs.availability.ListOverrides(ctx, doctor.ID, today, today.AddDate(0, 0, constant.OverrideHorizonDays))
	if err != nil {
		return nil, nil, fmt.Errorf("list availability overrides: %w", err)
	}

	return rules, overrides, nil
}

// Replace swaps the acting doctor's whole weekly schedule for the given rules.
// Blocks on the same day may not overlap, each must fit at least one slot, and
// each mode must be one the doctor offers.
func (avs *AvailabilityService) Replace(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	rules []models.AvailabilityRule,
) ([]*models.AvailabilityRule, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("replace availability context: %w", err)
	}

	doctor, err := avs.ownDoctor(ctx, actingUserID, actingRole)
	if err != nil {
		return nil, err
	}

	if len(rules) > constant.MaxAvailabilityRules {
		return nil, apperrors.Invalid(apperrors.ErrInvalidAvailabilityInput, "rules", fmt.Sprintf("must have at most %d blocks", constant.MaxAvailabilityRules))
	}
	for _, rule := range rules {
		if rule.DayOfWeek < time.Sunday || rule.DayOfWeek > time.Saturday {
			return nil, apperrors.Invalid(apperrors.ErrInvalidAvailabilityInput, "dayOfWeek", "must be 0 (Sunday) to 6 (Saturday)")
		}
		if err := validateMinuteRange(rule.StartMinute, rule.EndMinute, doctor.SlotDurationMin); err != nil {
			return nil, err
		}
		if !offersMode(doctor, rule.Mode) {
			return nil, apperrors.Invalid(apperrors.ErrInvalidAvailabilityInput, "mode", "must be one of the doctor's consultation modes")
		}
	}

	sorted := append([]models.AvailabilityRule(nil), rules...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].DayOfWeek != sorted[j].DayOfWeek {
			return sorted[i].DayOfWeek < sorted[j].DayOfWeek
		}
		return sorted[i].StartMinute < sorted[j].StartMinute
	})
	for index := 1; index < len(sorted); index++ {
		previous, current := sorted[index-1], sorted[index]
		if previous.DayOfWeek == current.DayOfWeek && current.StartMinute < previous.EndMinute {
			return nil, apperrors.Invalid(apperrors.ErrAvailabilityOverlap, "", fmt.Sprintf("%s blocks overlap", current.DayOfWeek))
		}
	}

	var replaced []*models.AvailabilityRule
	if err := avs.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		next, err := avs.availability.ReplaceRules(txCtx, doctor.ID, sorted)
		if err != nil {
			return err
		}
		replaced = next
		return nil
	}); err != nil {
		return nil, fmt.Errorf("replace availability rules: %w", err)
	}

	return replaced, nil
}

// AddOverride records a day off, a partial leave or an extra session for one
// date between today and a year ahead.
func (avs *AvailabilityService) AddOverride(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	override *models.AvailabilityOverride,
) (*models.AvailabilityOverride, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("add availability override context: %w", err)
	}
	if override == nil {
		return nil, apperrors.ErrNilOverrideRequest
	}

	doctor, err := avs.ownDoctor(ctx, actingUserID, actingRole)
	if err != nil {
		return nil, err
	}

	date := utils.CivilDate(override.Date)
	today := avs.today()
	if date.Before(today) || date.After(today.AddDate(0, 0, constant.OverrideHorizonDays)) {
		return nil, apperrors.Invalid(apperrors.ErrInvalidAvailabilityInput, "date", "must be between today and one year ahead")
	}

	hasTimes := override.StartMinute != nil && override.EndMinute != nil
	if (override.StartMinute == nil) != (override.EndMinute == nil) {
		return nil, apperrors.Invalid(apperrors.ErrInvalidAvailabilityInput, "", "give both startTime and endTime, or neither for a whole day off")
	}
	if override.IsAvailable && !hasTimes {
		return nil, apperrors.Invalid(apperrors.ErrInvalidAvailabilityInput, "", "an extra session needs startTime and endTime")
	}
	if hasTimes {
		minimumMinutes := 1
		if override.IsAvailable {
			minimumMinutes = doctor.SlotDurationMin
		}
		if err := validateMinuteRange(*override.StartMinute, *override.EndMinute, minimumMinutes); err != nil {
			return nil, err
		}
	}
	if override.IsAvailable && !offersMode(doctor, override.Mode) {
		return nil, apperrors.Invalid(apperrors.ErrInvalidAvailabilityInput, "mode", "must be one of the doctor's consultation modes")
	}
	if !override.IsAvailable {
		override.Mode = models.ConsultationModeInPerson
	}

	override.Reason = strings.TrimSpace(override.Reason)
	if len(override.Reason) > constant.MaxOverrideReasonLength {
		return nil, apperrors.Invalid(apperrors.ErrInvalidAvailabilityInput, "reason", fmt.Sprintf("must be at most %d characters", constant.MaxOverrideReasonLength))
	}

	override.ID = 0
	override.DoctorID = doctor.ID
	override.Date = date

	created, err := avs.availability.CreateOverride(ctx, override)
	if err != nil {
		return nil, fmt.Errorf("create availability override: %w", err)
	}

	return created, nil
}

// ListSlots returns the free slots of a verified doctor on one date, from today
// up to 90 days ahead. Slots that already started today or overlap a scheduled
// appointment are left out.
func (avs *AvailabilityService) ListSlots(
	ctx context.Context,
	doctorID uint64,
	date time.Time,
) ([]models.Slot, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list slots context: %w", err)
	}
	if doctorID == 0 {
		return nil, fmt.Errorf("list slots without doctor id: %w", apperrors.ErrInvalidDoctorInput)
	}

	date = utils.CivilDate(date)
	today := avs.today()
	if date.Before(today) || date.After(today.AddDate(0, 0, constant.SlotHorizonDays)) {
		return nil, apperrors.Invalid(apperrors.ErrInvalidAvailabilityInput, "date", fmt.Sprintf("must be between today and %d days ahead", constant.SlotHorizonDays))
	}

	doctor, err := avs.verifiedDoctor(ctx, doctorID)
	if err != nil {
		return nil, err
	}

	return avs.freeSlots(ctx, doctor, date)
}

// Timezone is the clinic timezone name that availability times are local to.
func (avs *AvailabilityService) Timezone() string {
	return avs.location.String()
}

func (avs *AvailabilityService) ownDoctor(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
) (*models.Doctor, error) {
	if err := validateDoctorActor(actingUserID, actingRole); err != nil {
		return nil, err
	}

	doctor, err := avs.doctors.GetByUserID(ctx, actingUserID)
	if err != nil {
		if errors.Is(err, apperrors.ErrDoctorNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("get own doctor profile: %w", err)
	}

	return doctor, nil
}

func (avs *AvailabilityService) verifiedDoctor(ctx context.Context, doctorID uint64) (*models.Doctor, error) {
	doctor, err := avs.doctors.GetByID(ctx, doctorID)
	if err != nil {
		if errors.Is(err, apperrors.ErrDoctorNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("get doctor for slots: %w", err)
	}
	if doctor.VerificationStatus != models.VerificationStatusVerified {
		return nil, apperrors.ErrDoctorNotFound
	}

	return doctor, nil
}

// freeSlots computes the doctor's slots on date and removes the ones taken by
// a scheduled appointment. A doctor who does not accept patients has none.
func (avs *AvailabilityService) freeSlots(
	ctx context.Context,
	doctor *models.Doctor,
	date time.Time,
) ([]models.Slot, error) {
	if !doctor.IsAcceptingPatients {
		return []models.Slot{}, nil
	}

	rules, err := avs.availability.ListRulesByDay(ctx, doctor.ID, date.Weekday())
	if err != nil {
		return nil, fmt.Errorf("list availability rules for slots: %w", err)
	}
	overrides, err := avs.availability.ListOverrides(ctx, doctor.ID, date, date)
	if err != nil {
		return nil, fmt.Errorf("list availability overrides for slots: %w", err)
	}

	dayStart := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, avs.location)
	booked, err := avs.appointments.ListByDoctorBetween(ctx, doctor.ID, dayStart, dayStart.AddDate(0, 0, 1))
	if err != nil {
		return nil, fmt.Errorf("list booked appointments for slots: %w", err)
	}

	slots := buildSlots(date, rules, overrides, doctor.SlotDurationMin, avs.location, avs.now())

	return withoutBooked(slots, booked), nil
}

func (avs *AvailabilityService) today() time.Time {
	return utils.CivilDate(avs.now().In(avs.location))
}
