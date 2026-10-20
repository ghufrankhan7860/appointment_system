package utils

import (
	"fmt"
	"time"

	"appointment_system/internal/apps/appointment/constant"
)

// ParseClockTime turns "HH:MM" into minutes from midnight. "24:00" is accepted
// as the end of the day.
func ParseClockTime(value string) (int, error) {
	if value == constant.EndOfDayClock {
		return constant.MinutesPerDay, nil
	}

	parsed, err := time.Parse(constant.ClockLayout, value)
	if err != nil {
		return 0, fmt.Errorf("time %q must be HH:MM", value)
	}

	return parsed.Hour()*constant.MinutesPerHour + parsed.Minute(), nil
}

// FormatClockTime turns minutes from midnight back into "HH:MM".
func FormatClockTime(minutes int) string {
	return fmt.Sprintf("%02d:%02d", minutes/constant.MinutesPerHour, minutes%constant.MinutesPerHour)
}
