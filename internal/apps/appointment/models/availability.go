package models

import "time"

// AvailabilityRule is one weekly working block. Minutes count from midnight in
// the clinic timezone: 540 is 09:00 and 1440 is the end of the day.
type AvailabilityRule struct {
	ID          uint64
	DoctorID    uint64
	DayOfWeek   time.Weekday
	StartMinute int
	EndMinute   int
	Mode        ConsultationMode
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// AvailabilityOverride changes the weekly rules for one date. Without times and
// not available it is a whole day off; with times it is a partial leave, or an
// extra session when available. Date holds only the calendar day.
type AvailabilityOverride struct {
	ID          uint64
	DoctorID    uint64
	Date        time.Time
	StartMinute *int
	EndMinute   *int
	IsAvailable bool
	Mode        ConsultationMode
	Reason      string
	CreatedAt   time.Time
}

type Slot struct {
	StartsAt time.Time
	EndsAt   time.Time
	Mode     ConsultationMode
}
