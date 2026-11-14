package models

import "time"

// PatientPass is the rotating code a patient shows at a clinic desk, as a QR
// token and a short typed code. A patient has at most one; only hashes are kept.
type PatientPass struct {
	ID              uint64
	PatientID       uint64
	CreatedByUserID uint64
	TokenHash       string
	CodeHash        string
	ExpiresAt       time.Time
	CreatedAt       time.Time
}

type ClinicPatientSource string

const (
	// ClinicPatientSourcePass: the patient showed their pass at the desk.
	ClinicPatientSourcePass ClinicPatientSource = "pass"
	// ClinicPatientSourceWalkIn: the desk added the patient, who has no app.
	ClinicPatientSourceWalkIn ClinicPatientSource = "walk_in"
)

// ClinicPatient records that a doctor's desk may book for a patient.
type ClinicPatient struct {
	DoctorID        uint64
	PatientID       uint64
	Source          ClinicPatientSource
	CreatedByUserID uint64
	CreatedAt       time.Time
}
