package admin

// SuspendDoctorResponse adds what the suspension did to the doctor's bookings
// that had not started: how many were cancelled and how many of those were
// paid and refunded (or queued for a manual payout).
type SuspendDoctorResponse struct {
	GetDoctorResponse
	CancelledAppointments int `json:"cancelledAppointments" example:"3"`
	RefundsRequested      int `json:"refundsRequested" example:"1"`
}
