package controller

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/dtos/patient"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
	"context"
	"time"
)

type PatientController struct {
	patientService *services.PatientService
}

func NewPatientController(patientService *services.PatientService) (*PatientController, error) {
	if patientService == nil {
		return nil, apperrors.ErrNilPatientService
	}
	return &PatientController{
		patientService: patientService,
	}, nil
}

func (pc *PatientController) Create(ctx context.Context, request *patient.CreatePatientRequest) (*patient.CreatePatientResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}

	dateOfBirth, err := time.Parse(constant.DateLayout, request.DateOfBirth)
	if err != nil {
		return nil, apperrors.Invalid(apperrors.ErrInvalidPatientInput, "dateOfBirth", "must be YYYY-MM-DD")
	}

	newPatient := models.Patient{
		Name:             request.Name,
		Gender:           models.Gender(request.Gender),
		DateOfBirth:      dateOfBirth,
		BloodType:        models.BloodType(request.BloodType),
		EmergencyContact: request.EmergencyContact,
	}
	createdPatient, err := pc.patientService.Create(ctx, userID, &newPatient, models.Relationship(request.Relationship))
	if err != nil {
		return nil, err
	}

	return &patient.CreatePatientResponse{
		ID:               createdPatient.ID,
		Name:             createdPatient.Name,
		Gender:           string(createdPatient.Gender),
		DateOfBirth:      createdPatient.DateOfBirth.Format(constant.DateLayout),
		BloodType:        string(createdPatient.BloodType),
		EmergencyContact: createdPatient.EmergencyContact,
	}, nil
}

func (pc *PatientController) GetByID(ctx context.Context, request *patient.GetPatientRequest) (*patient.GetPatientResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}

	foundPatient, err := pc.patientService.GetByID(ctx, userID, request.ID)
	if err != nil {
		return nil, err
	}

	return &patient.GetPatientResponse{
		ID:               foundPatient.ID,
		Name:             foundPatient.Name,
		Gender:           string(foundPatient.Gender),
		DateOfBirth:      foundPatient.DateOfBirth.Format(constant.DateLayout),
		BloodType:        string(foundPatient.BloodType),
		EmergencyContact: foundPatient.EmergencyContact,
	}, nil
}

func (pc *PatientController) List(ctx context.Context, request *patient.ListPatientsRequest) (*patient.ListPatientsResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}

	patients, err := pc.patientService.List(ctx, userID, request.Limit, request.Offset)
	if err != nil {
		return nil, err
	}

	response := patient.ListPatientsResponse{
		Patients: make([]patient.GetPatientResponse, 0, len(patients)),
	}
	for _, listedPatient := range patients {
		response.Patients = append(response.Patients, patient.GetPatientResponse{
			ID:               listedPatient.ID,
			Name:             listedPatient.Name,
			Gender:           string(listedPatient.Gender),
			DateOfBirth:      listedPatient.DateOfBirth.Format(constant.DateLayout),
			BloodType:        string(listedPatient.BloodType),
			EmergencyContact: listedPatient.EmergencyContact,
		})
	}

	return &response, nil
}

func (pc *PatientController) Update(ctx context.Context, request *patient.UpdatePatientRequest) (*patient.UpdatePatientResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}

	dateOfBirth, err := time.Parse(constant.DateLayout, request.DateOfBirth)
	if err != nil {
		return nil, apperrors.Invalid(apperrors.ErrInvalidPatientInput, "dateOfBirth", "must be YYYY-MM-DD")
	}

	changedPatient := models.Patient{
		ID:               request.ID,
		Name:             request.Name,
		Gender:           models.Gender(request.Gender),
		DateOfBirth:      dateOfBirth,
		BloodType:        models.BloodType(request.BloodType),
		EmergencyContact: request.EmergencyContact,
	}
	updatedPatient, err := pc.patientService.Update(ctx, userID, &changedPatient)
	if err != nil {
		return nil, err
	}

	return &patient.UpdatePatientResponse{
		ID:               updatedPatient.ID,
		Name:             updatedPatient.Name,
		Gender:           string(updatedPatient.Gender),
		DateOfBirth:      updatedPatient.DateOfBirth.Format(constant.DateLayout),
		BloodType:        string(updatedPatient.BloodType),
		EmergencyContact: updatedPatient.EmergencyContact,
	}, nil
}
