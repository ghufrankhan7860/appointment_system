package controller

import (
	"context"
	"fmt"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	dto "appointment_system/internal/apps/appointment/dtos/patient"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
)

type PatientController struct {
	patientService *services.PatientService
}

func NewPatientController(patientService *services.PatientService) (*PatientController, error) {
	if patientService == nil {
		return nil, apperrors.ErrNilPatientService
	}
	return &PatientController{patientService: patientService}, nil
}

func (pc *PatientController) Create(ctx context.Context, actingUserID uint64, request *dto.CreateRequest) (*dto.Response, error) {
	patient, err := toPatient(0, request.Name, request.Gender, request.DateOfBirth, request.BloodType, request.EmergencyContact)
	if err != nil {
		return nil, err
	}
	created, err := pc.patientService.Create(ctx, actingUserID, patient, models.Relationship(request.Relationship))
	if err != nil {
		return nil, err
	}
	return toPatientResponse(created), nil
}

func (pc *PatientController) Get(ctx context.Context, actingUserID, patientID uint64) (*dto.Response, error) {
	patient, err := pc.patientService.GetByID(ctx, actingUserID, patientID)
	if err != nil {
		return nil, err
	}
	return toPatientResponse(patient), nil
}

func (pc *PatientController) List(ctx context.Context, actingUserID uint64, limit, offset int) ([]*dto.Response, error) {
	patients, err := pc.patientService.List(ctx, actingUserID, limit, offset)
	if err != nil {
		return nil, err
	}
	responses := make([]*dto.Response, 0, len(patients))
	for _, patient := range patients {
		responses = append(responses, toPatientResponse(patient))
	}
	return responses, nil
}

func (pc *PatientController) Update(ctx context.Context, actingUserID, patientID uint64, request *dto.UpdateRequest) (*dto.Response, error) {
	patient, err := toPatient(patientID, request.Name, request.Gender, request.DateOfBirth, request.BloodType, request.EmergencyContact)
	if err != nil {
		return nil, err
	}
	updated, err := pc.patientService.Update(ctx, actingUserID, patient)
	if err != nil {
		return nil, err
	}
	return toPatientResponse(updated), nil
}

func toPatient(id uint64, name, gender, dateOfBirth, bloodType, emergencyContact string) (*models.Patient, error) {
	dob, err := time.Parse(dto.DateLayout, dateOfBirth)
	if err != nil {
		return nil, fmt.Errorf("%w: dateOfBirth must be YYYY-MM-DD", apperrors.ErrInvalidPatientInput)
	}
	return &models.Patient{
		ID:               id,
		Name:             name,
		Gender:           models.Gender(gender),
		DateOfBirth:      dob,
		BloodType:        models.BloodType(bloodType),
		EmergencyContact: emergencyContact,
	}, nil
}

func toPatientResponse(patient *models.Patient) *dto.Response {
	return &dto.Response{
		ID:               patient.ID,
		Name:             patient.Name,
		Gender:           string(patient.Gender),
		DateOfBirth:      patient.DateOfBirth.Format(dto.DateLayout),
		BloodType:        string(patient.BloodType),
		EmergencyContact: patient.EmergencyContact,
	}
}
