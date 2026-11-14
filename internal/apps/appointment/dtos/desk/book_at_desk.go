package desk

// BookAtDeskRequest books a free slot of the desk's doctor for a patient the
// desk has seen. The answer is appointment.AppointmentResponse.
type BookAtDeskRequest struct {
	PatientID uint64 `json:"patientId" binding:"required" example:"3"`
	StartsAt  string `json:"startsAt" binding:"required" example:"2026-10-26T09:00:00+05:30"`
	Mode      string `json:"mode" binding:"required" enums:"in_person,video,phone" example:"in_person"`
	Notes     string `json:"notes" example:"Fever since Monday"`

	IdempotencyKey string `json:"-" swaggerignore:"true"`
}
