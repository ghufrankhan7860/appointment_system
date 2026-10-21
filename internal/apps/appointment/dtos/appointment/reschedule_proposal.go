package appointment

import "time"

// RescheduleProposalResponse is a doctor's offer to move an appointment. The
// new time is held until expiresAt; NewDate and NewStartTime are clinic time.
type RescheduleProposalResponse struct {
	ID                       uint64    `json:"id"`
	AppointmentID            uint64    `json:"appointmentId"`
	PlaceholderAppointmentID uint64    `json:"placeholderAppointmentId"`
	Reason                   string    `json:"reason" enums:"emergency,personal,clinic_closed,other" example:"emergency"`
	Note                     string    `json:"note"`
	Status                   string    `json:"status" enums:"pending,accepted,declined,expired" example:"pending"`
	ExpiresAt                time.Time `json:"expiresAt" example:"2026-10-27T09:00:00+05:30"`
	NewStartsAt              time.Time `json:"newStartsAt" example:"2026-10-27T11:00:00+05:30"`
	NewEndsAt                time.Time `json:"newEndsAt" example:"2026-10-27T11:30:00+05:30"`
	NewDate                  string    `json:"newDate" example:"2026-10-27"`
	NewStartTime             string    `json:"newStartTime" example:"11:00"`
	NewMode                  string    `json:"newMode" example:"in_person"`
	CreatedAt                time.Time `json:"createdAt"`
}

type GetRescheduleProposalRequest struct {
	ID uint64 `uri:"id"`
}

// AnswerRescheduleProposalRequest identifies the appointment whose pending
// proposal the patient side accepts or declines.
type AnswerRescheduleProposalRequest struct {
	ID uint64 `uri:"id"`
}
