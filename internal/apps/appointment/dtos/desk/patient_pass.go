package desk

import "time"

type IssuePassRequest struct {
	ID uint64 `uri:"id" binding:"required"`
}

// PatientPassResponse is what the patient shows at the clinic desk: the QR
// carries Token (in a link to the desk), Code is typed at a desk without a
// camera. Both stop working at ExpiresAt or when a new pass is issued.
type PatientPassResponse struct {
	Token     string    `json:"token" example:"m2Rk9vQp0bW3xYzA1cDe4f"`
	Code      string    `json:"code" example:"K7Q2M9XD"`
	ExpiresAt time.Time `json:"expiresAt"`
}
