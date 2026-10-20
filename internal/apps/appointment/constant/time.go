package constant

import "time"

const (
	// DateLayout is the wire format for calendar dates: dateOfBirth, override and slot dates.
	DateLayout = time.DateOnly
	// ClockLayout is the wire format for times of day such as startTime and endTime.
	ClockLayout = "15:04"
	// EndOfDayClock is accepted as an endTime meaning midnight at the end of the day.
	EndOfDayClock = "24:00"

	MinutesPerHour = 60
	MinutesPerDay  = 24 * MinutesPerHour

	// DefaultTimezone is the clinic timezone when APP_TIMEZONE is not set.
	DefaultTimezone = "Asia/Kolkata"
)
