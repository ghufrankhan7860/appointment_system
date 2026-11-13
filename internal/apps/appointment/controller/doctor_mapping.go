package controller

import (
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/dtos/doctor"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/utils"
)

func toDoctorResponse(profile *models.Doctor) doctor.DoctorResponse {
	specialities := make([]doctor.SpecialityResponse, 0, len(profile.Specialities))
	for _, speciality := range profile.Specialities {
		specialities = append(specialities, doctor.SpecialityResponse{
			Slug:      speciality.Slug,
			Name:      speciality.Name,
			IsPrimary: speciality.IsPrimary,
		})
	}

	tags := make([]string, 0, len(profile.Tags))
	for _, tag := range profile.Tags {
		tags = append(tags, tag.Tag)
	}

	modes := make([]string, 0, len(profile.ConsultationModes))
	for _, mode := range profile.ConsultationModes {
		modes = append(modes, string(mode))
	}

	return doctor.DoctorResponse{
		ID:                  profile.ID,
		FullName:            profile.FullName,
		RegistrationNumber:  profile.RegistrationNumber,
		RegistrationCouncil: profile.RegistrationCouncil,
		RegistrationYear:    profile.RegistrationYear,
		Qualification:       profile.Qualification,
		YearsOfExperience:   profile.YearsOfExperience,
		Bio:                 profile.Bio,
		ConsultationFee:     profile.ConsultationFee,
		SlotDurationMin:     profile.SlotDurationMin,
		ConsultationModes:   modes,
		Specialities:        specialities,
		Tags:                tags,
		Clinic: doctor.ClinicResponse{
			Name:    profile.ClinicName,
			Address: profile.ClinicAddress,
			City:    profile.City,
			Pincode: profile.Pincode,
		},
		VerificationStatus:  string(profile.VerificationStatus),
		VerifiedAt:          profile.VerifiedAt,
		IsAcceptingPatients: profile.IsAcceptingPatients,
		UPIID:               profile.UPIID,
		HasUPIQR:            profile.UPIQRStorageKey != nil,
	}
}

func toConsultationModes(values []string) []models.ConsultationMode {
	modes := make([]models.ConsultationMode, 0, len(values))
	for _, value := range values {
		modes = append(modes, models.ConsultationMode(value))
	}

	return modes
}

func toDoctorTags(values []string) []models.DoctorTag {
	tags := make([]models.DoctorTag, 0, len(values))
	for _, value := range values {
		tags = append(tags, models.DoctorTag{Tag: value})
	}

	return tags
}

func toAvailabilityRuleResponse(rule *models.AvailabilityRule) doctor.AvailabilityRuleResponse {
	return doctor.AvailabilityRuleResponse{
		DayOfWeek: int(rule.DayOfWeek),
		Day:       rule.DayOfWeek.String(),
		StartTime: utils.FormatClockTime(rule.StartMinute),
		EndTime:   utils.FormatClockTime(rule.EndMinute),
		Mode:      string(rule.Mode),
	}
}

func toOverrideResponse(override *models.AvailabilityOverride) doctor.OverrideResponse {
	response := doctor.OverrideResponse{
		ID:          override.ID,
		Date:        override.Date.Format(constant.DateLayout),
		IsAvailable: override.IsAvailable,
		Mode:        string(override.Mode),
		Reason:      override.Reason,
	}
	if override.StartMinute != nil && override.EndMinute != nil {
		response.StartTime = utils.FormatClockTime(*override.StartMinute)
		response.EndTime = utils.FormatClockTime(*override.EndMinute)
	}

	return response
}
