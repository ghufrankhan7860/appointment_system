package utils

import "time"

// CivilDate keeps only the calendar day of value, as UTC midnight, so a date
// never shifts by a day through timezone conversion or a DATE column.
func CivilDate(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}
