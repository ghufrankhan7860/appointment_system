package appointment

type ListAppointmentsRequest struct {
	PatientID uint64 `form:"patientId"`
	Status    string `form:"status"`
	From      string `form:"from"`
	To        string `form:"to"`
	Limit     int    `form:"limit"`
	Offset    int    `form:"offset"`
}

type ListAppointmentsResponse struct {
	Appointments []AppointmentResponse `json:"appointments"`
}
