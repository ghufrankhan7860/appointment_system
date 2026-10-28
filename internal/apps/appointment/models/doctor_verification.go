package models

import "time"

// DoctorVerification records one verification status change and who made it.
type DoctorVerification struct {
	ID               uint64
	DoctorID         uint64
	ReviewedByUserID uint64
	FromStatus       VerificationStatus
	ToStatus         VerificationStatus
	Reason           string
	CreatedAt        time.Time
}

// SuspensionResult says what suspending a doctor did to their bookings:
// how many that had not started were cancelled, and how many of those were
// paid and so got a refund.
type SuspensionResult struct {
	CancelledAppointments int
	RefundsRequested      int
}
