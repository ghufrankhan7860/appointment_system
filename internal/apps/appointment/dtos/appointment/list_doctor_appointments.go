package appointment

type ListDoctorAppointmentsRequest struct {
	Date string `form:"date"`
}

type ListDoctorAppointmentsResponse struct {
	Date         string                `json:"date" example:"2026-10-26"`
	Timezone     string                `json:"timezone" example:"Asia/Kolkata"`
	Appointments []AppointmentResponse `json:"appointments"`
}
