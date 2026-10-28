package controller

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/dtos/doctor"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
	"appointment_system/internal/apps/appointment/utils"
	"context"
	"time"
)

type DoctorController struct {
	doctorService       *services.DoctorService
	availabilityService *services.AvailabilityService
}

func NewDoctorController(
	doctorService *services.DoctorService,
	availabilityService *services.AvailabilityService,
) (*DoctorController, error) {
	if doctorService == nil {
		return nil, apperrors.ErrNilDoctorService
	}
	if availabilityService == nil {
		return nil, apperrors.ErrNilAvailabilityService
	}
	return &DoctorController{
		doctorService:       doctorService,
		availabilityService: availabilityService,
	}, nil
}

func (dc *DoctorController) Create(ctx context.Context, request *doctor.CreateDoctorRequest) (*doctor.DoctorResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	newDoctor := models.Doctor{
		FullName:            request.FullName,
		RegistrationNumber:  request.RegistrationNumber,
		RegistrationCouncil: request.RegistrationCouncil,
		RegistrationYear:    request.RegistrationYear,
		Qualification:       request.Qualification,
		YearsOfExperience:   request.YearsOfExperience,
		Bio:                 request.Bio,
		ConsultationFee:     request.ConsultationFee,
		SlotDurationMin:     request.SlotDurationMin,
		ClinicName:          request.Clinic.Name,
		ClinicAddress:       request.Clinic.Address,
		City:                request.Clinic.City,
		Pincode:             request.Clinic.Pincode,
		ConsultationModes:   toConsultationModes(request.ConsultationModes),
		Tags:                toDoctorTags(request.Tags),
	}
	createdDoctor, err := dc.doctorService.Create(ctx, userID, role, &newDoctor, request.Specialities)
	if err != nil {
		return nil, err
	}

	response := toDoctorResponse(createdDoctor)

	return &response, nil
}

func (dc *DoctorController) GetMe(ctx context.Context) (*doctor.DoctorResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	foundDoctor, err := dc.doctorService.GetMe(ctx, userID, role)
	if err != nil {
		return nil, err
	}

	response := toDoctorResponse(foundDoctor)

	return &response, nil
}

func (dc *DoctorController) Update(ctx context.Context, request *doctor.UpdateDoctorRequest) (*doctor.DoctorResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	changedDoctor := models.Doctor{
		FullName:            request.FullName,
		RegistrationNumber:  request.RegistrationNumber,
		RegistrationCouncil: request.RegistrationCouncil,
		RegistrationYear:    request.RegistrationYear,
		Qualification:       request.Qualification,
		YearsOfExperience:   request.YearsOfExperience,
		Bio:                 request.Bio,
		ConsultationFee:     request.ConsultationFee,
		SlotDurationMin:     request.SlotDurationMin,
		ClinicName:          request.Clinic.Name,
		ClinicAddress:       request.Clinic.Address,
		City:                request.Clinic.City,
		Pincode:             request.Clinic.Pincode,
		ConsultationModes:   toConsultationModes(request.ConsultationModes),
		Tags:                toDoctorTags(request.Tags),
	}
	updatedDoctor, err := dc.doctorService.Update(ctx, userID, role, &changedDoctor, request.Specialities, request.IsAcceptingPatients)
	if err != nil {
		return nil, err
	}

	response := toDoctorResponse(updatedDoctor)

	return &response, nil
}

func (dc *DoctorController) GetByID(ctx context.Context, request *doctor.GetDoctorRequest) (*doctor.DoctorResponse, error) {
	foundDoctor, err := dc.doctorService.GetByID(ctx, request.ID)
	if err != nil {
		return nil, err
	}

	response := toDoctorResponse(foundDoctor)

	return &response, nil
}

func (dc *DoctorController) List(ctx context.Context, request *doctor.ListDoctorsRequest) (*doctor.ListDoctorsResponse, error) {
	search := models.DoctorSearch{
		Query:        request.Query,
		Specialities: request.Specialities,
		Tags:         request.Tags,
		City:         request.City,
		Mode:         models.ConsultationMode(request.Mode),
		FeeMax:       request.FeeMax,
		Limit:        request.Limit,
		Offset:       request.Offset,
	}
	if request.Date != "" {
		date, err := time.Parse(constant.DateLayout, request.Date)
		if err != nil {
			return nil, apperrors.Invalid(apperrors.ErrInvalidAvailabilityInput, "date", "must be YYYY-MM-DD")
		}
		search.Date = &date
	}

	doctors, err := dc.doctorService.List(ctx, search)
	if err != nil {
		return nil, err
	}

	response := doctor.ListDoctorsResponse{
		Doctors: make([]doctor.DoctorResponse, 0, len(doctors)),
	}
	for _, listedDoctor := range doctors {
		response.Doctors = append(response.Doctors, toDoctorResponse(listedDoctor))
	}

	return &response, nil
}

func (dc *DoctorController) ListSpecialities(ctx context.Context) (*doctor.ListSpecialitiesResponse, error) {
	specialities, err := dc.doctorService.ListSpecialities(ctx)
	if err != nil {
		return nil, err
	}

	response := doctor.ListSpecialitiesResponse{
		Specialities: make([]doctor.SpecialityResponse, 0, len(specialities)),
	}
	for _, speciality := range specialities {
		response.Specialities = append(response.Specialities, doctor.SpecialityResponse{
			Slug: speciality.Slug,
			Name: speciality.Name,
		})
	}

	return &response, nil
}

func (dc *DoctorController) GetAvailability(ctx context.Context) (*doctor.GetAvailabilityResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	rules, overrides, err := dc.availabilityService.Get(ctx, userID, role)
	if err != nil {
		return nil, err
	}

	response := doctor.GetAvailabilityResponse{
		Timezone:  dc.availabilityService.Timezone(),
		Rules:     make([]doctor.AvailabilityRuleResponse, 0, len(rules)),
		Overrides: make([]doctor.OverrideResponse, 0, len(overrides)),
	}
	for _, rule := range rules {
		response.Rules = append(response.Rules, toAvailabilityRuleResponse(rule))
	}
	for _, override := range overrides {
		response.Overrides = append(response.Overrides, toOverrideResponse(override))
	}

	return &response, nil
}

func (dc *DoctorController) ReplaceAvailability(ctx context.Context, request *doctor.ReplaceAvailabilityRequest) (*doctor.ReplaceAvailabilityResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	rules := make([]models.AvailabilityRule, 0, len(request.Rules))
	for _, ruleRequest := range request.Rules {
		startMinute, err := utils.ParseClockTime(ruleRequest.StartTime)
		if err != nil {
			return nil, apperrors.Invalid(apperrors.ErrInvalidAvailabilityInput, "startTime", "must be HH:MM")
		}
		endMinute, err := utils.ParseClockTime(ruleRequest.EndTime)
		if err != nil {
			return nil, apperrors.Invalid(apperrors.ErrInvalidAvailabilityInput, "endTime", "must be HH:MM")
		}
		rules = append(rules, models.AvailabilityRule{
			DayOfWeek:   time.Weekday(ruleRequest.DayOfWeek),
			StartMinute: startMinute,
			EndMinute:   endMinute,
			Mode:        models.ConsultationMode(ruleRequest.Mode),
		})
	}

	replacedRules, err := dc.availabilityService.Replace(ctx, userID, role, rules)
	if err != nil {
		return nil, err
	}

	response := doctor.ReplaceAvailabilityResponse{
		Rules: make([]doctor.AvailabilityRuleResponse, 0, len(replacedRules)),
	}
	for _, rule := range replacedRules {
		response.Rules = append(response.Rules, toAvailabilityRuleResponse(rule))
	}

	return &response, nil
}

func (dc *DoctorController) AddOverride(ctx context.Context, request *doctor.CreateOverrideRequest) (*doctor.OverrideResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	date, err := time.Parse(constant.DateLayout, request.Date)
	if err != nil {
		return nil, apperrors.Invalid(apperrors.ErrInvalidAvailabilityInput, "date", "must be YYYY-MM-DD")
	}

	newOverride := models.AvailabilityOverride{
		Date:        date,
		IsAvailable: request.IsAvailable,
		Mode:        models.ConsultationMode(request.Mode),
		Reason:      request.Reason,
	}
	if request.StartTime != "" || request.EndTime != "" {
		startMinute, err := utils.ParseClockTime(request.StartTime)
		if err != nil {
			return nil, apperrors.Invalid(apperrors.ErrInvalidAvailabilityInput, "startTime", "must be HH:MM")
		}
		endMinute, err := utils.ParseClockTime(request.EndTime)
		if err != nil {
			return nil, apperrors.Invalid(apperrors.ErrInvalidAvailabilityInput, "endTime", "must be HH:MM")
		}
		newOverride.StartMinute = &startMinute
		newOverride.EndMinute = &endMinute
	}

	createdOverride, err := dc.availabilityService.AddOverride(ctx, userID, role, &newOverride)
	if err != nil {
		return nil, err
	}

	response := toOverrideResponse(createdOverride)

	return &response, nil
}

func (dc *DoctorController) ListSlots(ctx context.Context, request *doctor.ListSlotsRequest) (*doctor.ListSlotsResponse, error) {
	date, err := time.Parse(constant.DateLayout, request.Date)
	if err != nil {
		return nil, apperrors.Invalid(apperrors.ErrInvalidAvailabilityInput, "date", "must be YYYY-MM-DD")
	}

	slots, err := dc.availabilityService.ListSlots(ctx, request.ID, date)
	if err != nil {
		return nil, err
	}

	response := doctor.ListSlotsResponse{
		Date:     date.Format(constant.DateLayout),
		Timezone: dc.availabilityService.Timezone(),
		Slots:    make([]doctor.SlotResponse, 0, len(slots)),
	}
	for _, slot := range slots {
		response.Slots = append(response.Slots, doctor.SlotResponse{
			StartsAt:  slot.StartsAt,
			EndsAt:    slot.EndsAt,
			StartTime: slot.StartsAt.Format(constant.ClockLayout),
			Mode:      string(slot.Mode),
		})
	}

	return &response, nil
}
