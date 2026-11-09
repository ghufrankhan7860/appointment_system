package controller

import (
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/dtos/patient"
	"appointment_system/internal/apps/appointment/models"
	"strings"
)

func toPatientResponse(linkedPatient *models.LinkedPatient) patient.GetPatientResponse {
	return patient.GetPatientResponse{
		ID:               linkedPatient.Patient.ID,
		Name:             linkedPatient.Patient.Name,
		Gender:           string(linkedPatient.Patient.Gender),
		DateOfBirth:      linkedPatient.Patient.DateOfBirth.Format(constant.DateLayout),
		BloodType:        string(linkedPatient.Patient.BloodType),
		EmergencyContact: linkedPatient.Patient.EmergencyContact,
		Relationship:     string(linkedPatient.Link.PatientRelationship),
		AccessLevel:      string(linkedPatient.Link.AccessLevel),
	}
}

func toPatientInviteResponse(invite *models.PatientInvite, code string) patient.PatientInviteResponse {
	return patient.PatientInviteResponse{
		ID:           invite.ID,
		Code:         code,
		Relationship: string(invite.PatientRelationship),
		AccessLevel:  string(invite.AccessLevel),
		ExpiresAt:    invite.ExpiresAt.UTC(),
		CreatedAt:    invite.CreatedAt.UTC(),
	}
}

// maskMobile keeps the first two and last two digits, enough to recognise a
// number without exposing it.
func maskMobile(mobileNo string) string {
	if len(mobileNo) <= 4 {
		return strings.Repeat("*", len(mobileNo))
	}

	return mobileNo[:2] + strings.Repeat("*", len(mobileNo)-4) + mobileNo[len(mobileNo)-2:]
}
