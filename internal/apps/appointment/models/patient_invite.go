package models

import "time"

type PatientInviteStatus string

const (
	PatientInviteStatusPending  PatientInviteStatus = "pending"
	PatientInviteStatusRedeemed PatientInviteStatus = "redeemed"
	PatientInviteStatusRevoked  PatientInviteStatus = "revoked"
	PatientInviteStatusExpired  PatientInviteStatus = "expired"
)

// PatientInvite lets a full holder share a patient: whoever redeems its code
// gets a user-patient link with PatientRelationship and AccessLevel. Only the
// sha256 of the code is kept.
type PatientInvite struct {
	ID                  uint64
	PatientID           uint64
	CreatedByUserID     uint64
	CodeHash            string
	PatientRelationship Relationship
	AccessLevel         AccessLevel
	Status              PatientInviteStatus
	ExpiresAt           time.Time
	RedeemedByUserID    *uint64
	RedeemedAt          *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}
