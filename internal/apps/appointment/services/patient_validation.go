package services

import (
	"strings"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
)

func validatePatientDemographics(patient *models.Patient) error {
	if strings.TrimSpace(patient.Name) == "" {
		return apperrors.Invalid(apperrors.ErrInvalidPatientInput, "name", "is required")
	}
	if patient.DateOfBirth.IsZero() {
		return apperrors.Invalid(apperrors.ErrInvalidPatientInput, "dateOfBirth", "is required")
	}
	if patient.DateOfBirth.After(time.Now().UTC()) {
		return apperrors.Invalid(apperrors.ErrInvalidPatientInput, "dateOfBirth", "cannot be in the future")
	}
	if !validGender(patient.Gender) {
		return apperrors.Invalid(apperrors.ErrInvalidPatientInput, "gender", "must be one of Male, Female, Other, Unknown")
	}
	if !validBloodType(patient.BloodType) {
		return apperrors.Invalid(apperrors.ErrInvalidPatientInput, "bloodType", "must be one of A+, A-, B+, B-, AB+, AB-, O+, O-")
	}
	return nil
}

func validGender(gender models.Gender) bool {
	switch gender {
	case models.GenderMale, models.GenderFemale, models.GenderOther, models.GenderUnknown:
		return true
	default:
		return false
	}
}

func validBloodType(bloodType models.BloodType) bool {
	switch bloodType {
	case models.BloodTypeAPlus, models.BloodTypeAMinus,
		models.BloodTypeBPlus, models.BloodTypeBMinus,
		models.BloodTypeABPlus, models.BloodTypeABMinus,
		models.BloodTypeOPlus, models.BloodTypeOMinus:
		return true
	default:
		return false
	}
}

func validRelationship(relationship models.Relationship) bool {
	switch relationship {
	case models.RelationshipSelf, models.RelationshipFather,
		models.RelationshipMother, models.RelationshipSpouse,
		models.RelationshipChild, models.RelationshipSibling,
		models.RelationshipGuardian, models.RelationshipCaregiver,
		models.RelationshipOther:
		return true
	default:
		return false
	}
}
