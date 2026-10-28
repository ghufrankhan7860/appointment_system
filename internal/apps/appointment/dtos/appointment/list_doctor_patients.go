package appointment

import "time"

type ListDoctorPatientsRequest struct {
	Query  string `form:"q"`
	Limit  int    `form:"limit"`
	Offset int    `form:"offset"`
}

type ListDoctorPatientsResponse struct {
	Patients []DoctorPatientResponse `json:"patients"`
}

// DoctorPatientResponse is one patient in the doctor's list. Visits counts
// completed, no-show and paid upcoming bookings; the times carry the clinic
// UTC offset.
type DoctorPatientResponse struct {
	ID                uint64     `json:"id"`
	Name              string     `json:"name" example:"Asha Verma"`
	Age               int        `json:"age" example:"34"`
	Gender            string     `json:"gender" example:"Female"`
	LastAppointmentAt *time.Time `json:"lastAppointmentAt,omitempty" example:"2026-10-20T09:00:00+05:30"`
	NextAppointmentAt *time.Time `json:"nextAppointmentAt,omitempty" example:"2026-11-02T09:00:00+05:30"`
	Visits            int        `json:"visits" example:"3"`
}
