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
