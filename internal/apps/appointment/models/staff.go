package models

import "time"

type StaffStatus string

const (
	StaffStatusActive  StaffStatus = "active"
	StaffStatusRemoved StaffStatus = "removed"
)

// DoctorStaff links a staff account to the one doctor it works for. Removing
// it ends the staff member's access; the row stays as history.
type DoctorStaff struct {
	ID              uint64
	DoctorID        uint64
	UserID          uint64
	Status          StaffStatus
	RemovedAt       *time.Time
	RemovedByUserID *uint64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// StaffMember is a staff link with the account's contact details, for the
// doctor's staff list.
type StaffMember struct {
	DoctorStaff
	Name     string
	Email    string
	MobileNo string
}

type StaffInviteStatus string

const (
	StaffInviteStatusPending  StaffInviteStatus = "pending"
	StaffInviteStatusRedeemed StaffInviteStatus = "redeemed"
	StaffInviteStatusRevoked  StaffInviteStatus = "revoked"
	StaffInviteStatusExpired  StaffInviteStatus = "expired"
)

// StaffInvite lets a doctor add a staff member: whoever signs up or joins with
// its code works for DoctorID. Only the sha256 of the code is kept.
type StaffInvite struct {
	ID               uint64
	DoctorID         uint64
	CreatedByUserID  uint64
	CodeHash         string
	Status           StaffInviteStatus
	ExpiresAt        time.Time
	RedeemedByUserID *uint64
	RedeemedAt       *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
