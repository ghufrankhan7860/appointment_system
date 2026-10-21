package services

import (
	"fmt"
	"sort"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
)

type minuteRange struct {
	start int
	end   int
}

func buildSlots(
	date time.Time,
	rules []*models.AvailabilityRule,
	overrides []*models.AvailabilityOverride,
	slotDuration int,
	location *time.Location,
	now time.Time,
) []models.Slot {
	slots := []models.Slot{}
	if slotDuration <= 0 {
		return slots
	}

	blocks := map[models.ConsultationMode][]minuteRange{}
	for _, rule := range rules {
		blocks[rule.Mode] = append(blocks[rule.Mode], minuteRange{start: rule.StartMinute, end: rule.EndMinute})
	}

	var leaves []minuteRange
	for _, override := range overrides {
		if override.StartMinute == nil || override.EndMinute == nil {
			if !override.IsAvailable {
				return slots
			}
			continue
		}
		block := minuteRange{start: *override.StartMinute, end: *override.EndMinute}
		if override.IsAvailable {
			blocks[override.Mode] = append(blocks[override.Mode], block)
		} else {
			leaves = append(leaves, block)
		}
	}

	dayStart := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, location)
	for _, mode := range []models.ConsultationMode{models.ConsultationModeInPerson, models.ConsultationModeVideo, models.ConsultationModePhone} {
		for _, block := range subtractMinuteRanges(mergeMinuteRanges(blocks[mode]), leaves) {
			for start := block.start; start+slotDuration <= block.end; start += slotDuration {
				startsAt := dayStart.Add(time.Duration(start) * time.Minute)
				if startsAt.Before(now) {
					continue
				}
				slots = append(slots, models.Slot{
					StartsAt: startsAt,
					EndsAt:   startsAt.Add(time.Duration(slotDuration) * time.Minute),
					Mode:     mode,
				})
			}
		}
	}

	sort.SliceStable(slots, func(i, j int) bool {
		return slots[i].StartsAt.Before(slots[j].StartsAt)
	})
	return slots
}

// withoutBooked drops every slot that overlaps a scheduled appointment or a
// time held by a reschedule proposal. It compares instants, so it still works
// after the doctor changes the slot length.
func withoutBooked(slots []models.Slot, booked []*models.AppointmentDetails) []models.Slot {
	free := make([]models.Slot, 0, len(slots))
	for _, slot := range slots {
		taken := false
		for _, appointment := range booked {
			holdsSlot := appointment.Status == models.AppointmentStatusScheduled ||
				appointment.Status == models.AppointmentStatusRescheduleProposed
			if holdsSlot &&
				slot.StartsAt.Before(appointment.EndsAt) && appointment.StartsAt.Before(slot.EndsAt) {
				taken = true
				break
			}
		}
		if !taken {
			free = append(free, slot)
		}
	}

	return free
}

func mergeMinuteRanges(ranges []minuteRange) []minuteRange {
	if len(ranges) == 0 {
		return nil
	}

	sorted := append([]minuteRange(nil), ranges...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].start < sorted[j].start })

	merged := []minuteRange{sorted[0]}
	for _, next := range sorted[1:] {
		last := &merged[len(merged)-1]
		if next.start <= last.end {
			if next.end > last.end {
				last.end = next.end
			}
			continue
		}
		merged = append(merged, next)
	}
	return merged
}

func subtractMinuteRanges(ranges []minuteRange, cuts []minuteRange) []minuteRange {
	result := ranges
	for _, cut := range cuts {
		next := make([]minuteRange, 0, len(result)+1)
		for _, block := range result {
			if cut.end <= block.start || cut.start >= block.end {
				next = append(next, block)
				continue
			}
			if cut.start > block.start {
				next = append(next, minuteRange{start: block.start, end: cut.start})
			}
			if cut.end < block.end {
				next = append(next, minuteRange{start: cut.end, end: block.end})
			}
		}
		result = next
	}
	return result
}

func validateMinuteRange(startMinute int, endMinute int, minimumMinutes int) error {
	if startMinute < 0 || endMinute > constant.MinutesPerDay || startMinute >= endMinute {
		return apperrors.Invalid(apperrors.ErrInvalidAvailabilityInput, "endTime", "must be after startTime")
	}
	if endMinute-startMinute < minimumMinutes {
		return apperrors.Invalid(apperrors.ErrInvalidAvailabilityInput, "", fmt.Sprintf("each time range must be at least %d minutes long", minimumMinutes))
	}

	return nil
}

func offersMode(doctor *models.Doctor, mode models.ConsultationMode) bool {
	for _, offered := range doctor.ConsultationModes {
		if offered == mode {
			return true
		}
	}

	return false
}
