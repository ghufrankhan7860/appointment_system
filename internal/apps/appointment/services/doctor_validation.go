package services

import (
	"fmt"
	"strings"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
)

func validateDoctorActor(actingUserID uint64, actingRole models.Role) error {
	if actingUserID == 0 {
		return fmt.Errorf("doctor action without acting user: %w", apperrors.ErrInvalidDoctorInput)
	}
	if actingRole != models.RoleDoctor {
		return apperrors.ErrDoctorRoleRequired
	}

	return nil
}

func normalizeDoctorProfile(doctor *models.Doctor) error {
	doctor.FullName = strings.TrimSpace(doctor.FullName)
	doctor.RegistrationNumber = strings.TrimSpace(doctor.RegistrationNumber)
	doctor.RegistrationCouncil = strings.TrimSpace(doctor.RegistrationCouncil)
	doctor.Qualification = strings.TrimSpace(doctor.Qualification)
	doctor.Bio = strings.TrimSpace(doctor.Bio)
	doctor.ClinicName = strings.TrimSpace(doctor.ClinicName)
	doctor.ClinicAddress = strings.TrimSpace(doctor.ClinicAddress)
	doctor.City = strings.TrimSpace(doctor.City)
	doctor.Pincode = strings.TrimSpace(doctor.Pincode)
	if doctor.SlotDurationMin == 0 {
		doctor.SlotDurationMin = constant.DefaultSlotDurationMin
	}

	if doctor.FullName == "" || doctor.RegistrationNumber == "" ||
		doctor.RegistrationCouncil == "" || doctor.Qualification == "" {
		return apperrors.Invalid(apperrors.ErrInvalidDoctorInput, "", "fullName, registrationNumber, registrationCouncil and qualification are required")
	}
	if doctor.RegistrationYear != 0 &&
		(doctor.RegistrationYear < constant.MinRegistrationYear || doctor.RegistrationYear > time.Now().UTC().Year()) {
		return apperrors.Invalid(apperrors.ErrInvalidDoctorInput, "registrationYear", fmt.Sprintf("must be between %d and the current year", constant.MinRegistrationYear))
	}
	if doctor.YearsOfExperience < 0 || doctor.ConsultationFee < 0 {
		return apperrors.Invalid(apperrors.ErrInvalidDoctorInput, "", "yearsOfExperience and consultationFee cannot be negative")
	}
	if doctor.SlotDurationMin < constant.MinSlotDurationMin || doctor.SlotDurationMin > constant.MaxSlotDurationMin {
		return apperrors.Invalid(apperrors.ErrInvalidDoctorInput, "slotDurationMin", fmt.Sprintf("must be between %d and %d", constant.MinSlotDurationMin, constant.MaxSlotDurationMin))
	}

	modes := make([]models.ConsultationMode, 0, len(doctor.ConsultationModes))
	seenModes := make(map[models.ConsultationMode]bool, len(doctor.ConsultationModes))
	for _, mode := range doctor.ConsultationModes {
		if !validConsultationMode(mode) {
			return apperrors.Invalid(apperrors.ErrInvalidDoctorInput, "consultationModes", "must only contain in_person, video or phone")
		}
		if !seenModes[mode] {
			seenModes[mode] = true
			modes = append(modes, mode)
		}
	}
	if len(modes) == 0 {
		return apperrors.Invalid(apperrors.ErrInvalidDoctorInput, "consultationModes", "must have at least one mode")
	}
	doctor.ConsultationModes = modes

	tags := make([]models.DoctorTag, 0, len(doctor.Tags))
	seenTags := make(map[string]bool, len(doctor.Tags))
	for _, tag := range doctor.Tags {
		value := strings.ToLower(strings.TrimSpace(tag.Tag))
		if value == "" || seenTags[value] {
			continue
		}
		if len(value) > constant.MaxDoctorTagLength {
			return apperrors.Invalid(apperrors.ErrInvalidDoctorInput, "tags", fmt.Sprintf("must each be at most %d characters", constant.MaxDoctorTagLength))
		}
		seenTags[value] = true
		tags = append(tags, models.DoctorTag{Tag: value, TagType: models.TagTypeKeyword})
	}
	if len(tags) > constant.MaxDoctorTags {
		return apperrors.Invalid(apperrors.ErrInvalidDoctorInput, "tags", fmt.Sprintf("must have at most %d items", constant.MaxDoctorTags))
	}
	doctor.Tags = tags

	return nil
}

func validConsultationMode(mode models.ConsultationMode) bool {
	switch mode {
	case models.ConsultationModeInPerson, models.ConsultationModeVideo, models.ConsultationModePhone:
		return true
	default:
		return false
	}
}
