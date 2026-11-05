package record

import "time"

// MedicalRecordResponse is the shape every record endpoint returns.
// Attachments are filled when one record is read.
type MedicalRecordResponse struct {
	ID                uint64               `json:"id"`
	AppointmentID     uint64               `json:"appointmentId"`
	PatientID         uint64               `json:"patientId"`
	DoctorID          uint64               `json:"doctorId"`
	ChiefComplaint    string               `json:"chiefComplaint" example:"Fever for three days"`
	Symptoms          string               `json:"symptoms" example:"Fever, body ache"`
	Diagnosis         string               `json:"diagnosis" example:"Viral fever"`
	ExaminationNotes  string               `json:"examinationNotes"`
	Advice            string               `json:"advice" example:"Rest and fluids"`
	PrescriptionNotes string               `json:"prescriptionNotes"`
	FollowUpOn        string               `json:"followUpOn,omitempty" example:"2026-11-02"`
	Vitals            Vitals               `json:"vitals"`
	Medicines         []Medicine           `json:"medicines"`
	Status            string               `json:"status" enums:"draft,finalized" example:"draft"`
	FinalizedAt       *time.Time           `json:"finalizedAt,omitempty"`
	CreatedAt         time.Time            `json:"createdAt"`
	UpdatedAt         time.Time            `json:"updatedAt"`
	Attachments       []AttachmentResponse `json:"attachments,omitempty"`
}

// Vitals are the measurements taken at the visit; a missing value was not measured.
type Vitals struct {
	BloodPressure         string   `json:"bloodPressure,omitempty" example:"120/80"`
	PulsePerMinute        *int     `json:"pulsePerMinute,omitempty" example:"82"`
	TemperatureFahrenheit *float64 `json:"temperatureFahrenheit,omitempty" example:"100.8"`
	WeightKg              *float64 `json:"weightKg,omitempty" example:"64.5"`
	SpO2Percent           *int     `json:"spo2Percent,omitempty" example:"98"`
}

// Medicine is one line of the visit's prescription.
type Medicine struct {
	MedicineName string `json:"medicineName" example:"Paracetamol"`
	Strength     string `json:"strength" example:"500 mg"`
	Form         string `json:"form" example:"tablet"`
	Dosage       string `json:"dosage" example:"1-0-1"`
	Frequency    string `json:"frequency" example:"twice daily"`
	DurationDays int    `json:"durationDays" example:"3"`
	Timing       string `json:"timing" enums:"before_food,after_food,with_food,any" example:"after_food"`
	Instructions string `json:"instructions"`
}

// AttachmentResponse describes an uploaded file. The file itself is read from
// GET /attachments/{id}/content.
type AttachmentResponse struct {
	ID              uint64    `json:"id"`
	PatientID       uint64    `json:"patientId"`
	MedicalRecordID *uint64   `json:"medicalRecordId,omitempty"`
	Kind            string    `json:"kind" enums:"lab_report,imaging,prescription_scan,referral,other" example:"lab_report"`
	Title           string    `json:"title" example:"Complete blood count"`
	ContentType     string    `json:"contentType" example:"application/pdf"`
	SizeBytes       int64     `json:"sizeBytes" example:"48213"`
	TakenOn         string    `json:"takenOn,omitempty" example:"2026-10-20"`
	CreatedAt       time.Time `json:"createdAt"`
}
