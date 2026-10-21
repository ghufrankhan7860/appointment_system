package models

import "time"

type ProposalReason string

const (
	ProposalReasonEmergency    ProposalReason = "emergency"
	ProposalReasonPersonal     ProposalReason = "personal"
	ProposalReasonClinicClosed ProposalReason = "clinic_closed"
	ProposalReasonOther        ProposalReason = "other"
)

type ProposalStatus string

const (
	ProposalStatusPending  ProposalStatus = "pending"
	ProposalStatusAccepted ProposalStatus = "accepted"
	ProposalStatusDeclined ProposalStatus = "declined"
	ProposalStatusExpired  ProposalStatus = "expired"
)

// RescheduleProposal is a doctor's offer to move an appointment. The offered
// time is held by a placeholder appointment in status reschedule_proposed, so
// both slots stay taken until the patient side answers or the offer expires.
// NewStartsAt, NewEndsAt and NewMode are read from the placeholder.
type RescheduleProposal struct {
	ID                       uint64
	AppointmentID            uint64
	PlaceholderAppointmentID uint64
	ProposedByUserID         uint64
	Reason                   ProposalReason
	Note                     string
	Status                   ProposalStatus
	ExpiresAt                time.Time
	RespondedAt              *time.Time
	RespondedByUserID        uint64
	NewStartsAt              time.Time
	NewEndsAt                time.Time
	NewMode                  ConsultationMode
	CreatedAt                time.Time
	UpdatedAt                time.Time
}
