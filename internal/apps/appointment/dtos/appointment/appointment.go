package appointment

import "time"

// AppointmentResponse is the shape every appointment endpoint returns.
// StartsAt and EndsAt carry the clinic UTC offset; Date and StartTime are the
// same moment as clinic wall-clock values.
type AppointmentResponse struct {
	ID              uint64          `json:"id"`
	StartsAt        time.Time       `json:"startsAt" example:"2026-10-26T09:00:00+05:30"`
	EndsAt          time.Time       `json:"endsAt" example:"2026-10-26T09:30:00+05:30"`
	Date            string          `json:"date" example:"2026-10-26"`
	StartTime       string          `json:"startTime" example:"09:00"`
	Mode            string          `json:"mode" example:"in_person"`
	Status          string          `json:"status" enums:"scheduled,completed,cancelled,no_show,reschedule_proposed" example:"scheduled"`
	ConsultationFee int             `json:"consultationFee" example:"50000"`
	SlotDurationMin int             `json:"slotDurationMin" example:"30"`
	Notes           string          `json:"notes" example:"Fever since Monday"`
	CancelledAt     *time.Time      `json:"cancelledAt,omitempty"`
	CancelReason    string          `json:"cancelReason,omitempty" enums:"patient_request,doctor_request,admin,rescheduled,doctor_suspended,payment_expired,proposal_declined,proposal_expired,payment_unverified,payment_rejected"`
	RescheduledFrom *uint64         `json:"rescheduledFromId,omitempty" example:"49"`
	PaymentStatus   string          `json:"paymentStatus" enums:"unpaid,pending_verification,due_at_clinic,paid,refunded,waived" example:"unpaid"`
	PaymentDueAt    *time.Time      `json:"paymentDueAt,omitempty"`
	BookedVia       string          `json:"bookedVia" enums:"app,desk" example:"app"`
	Doctor          DoctorSummary   `json:"doctor"`
	Patient         PatientSummary  `json:"patient"`
	BookedBy        BookedBySummary `json:"bookedBy"`
}

type DoctorSummary struct {
	ID         uint64 `json:"id"`
	Name       string `json:"name" example:"Dr. Meera Iyer"`
	Speciality string `json:"speciality" example:"Cardiology"`
}

type PatientSummary struct {
	ID   uint64 `json:"id"`
	Name string `json:"name" example:"Asha Verma"`
}

// BookedBySummary names who booked. PatientRelationship is the booker's
// relationship to the patient, and empty when the booker has no link to them.
type BookedBySummary struct {
	UserID              uint64 `json:"userId"`
	PatientRelationship string `json:"patientRelationship,omitempty" example:"child"`
}
