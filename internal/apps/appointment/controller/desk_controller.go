package controller

import (
	"context"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/dtos/appointment"
	"appointment_system/internal/apps/appointment/dtos/desk"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
)

type DeskController struct {
	deskService        *services.DeskService
	appointmentService *services.AppointmentService
}

func NewDeskController(deskService *services.DeskService, appointmentService *services.AppointmentService) (*DeskController, error) {
	if deskService == nil {
		return nil, apperrors.ErrNilDeskService
	}
	if appointmentService == nil {
		return nil, apperrors.ErrNilAppointmentService
	}

	return &DeskController{deskService: deskService, appointmentService: appointmentService}, nil
}

func (dc *DeskController) IssuePass(ctx context.Context, request *desk.IssuePassRequest) (*desk.PatientPassResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}

	pass, token, code, err := dc.deskService.IssuePass(ctx, userID, request.ID)
	if err != nil {
		return nil, err
	}

	return &desk.PatientPassResponse{Token: token, Code: code, ExpiresAt: pass.ExpiresAt}, nil
}

func (dc *DeskController) LookupPass(ctx context.Context, request *desk.LookupPassRequest) (*desk.DeskPatientResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	patient, err := dc.deskService.LookupPass(ctx, userID, role, request.Code)
	if err != nil {
		return nil, err
	}

	response := toDeskPatientResponse(patient)

	return &response, nil
}

func (dc *DeskController) CreateWalkIn(ctx context.Context, request *desk.CreateWalkInRequest) (*desk.DeskPatientResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	dateOfBirth, err := time.Parse(constant.DateLayout, request.DateOfBirth)
	if err != nil {
		return nil, apperrors.Invalid(apperrors.ErrInvalidPatientInput, "dateOfBirth", "must be YYYY-MM-DD")
	}

	createdPatient, err := dc.deskService.CreateWalkIn(ctx, userID, role, &models.Patient{
		Name:             request.Name,
		Gender:           models.Gender(request.Gender),
		DateOfBirth:      dateOfBirth,
		BloodType:        models.BloodType(request.BloodType),
		EmergencyContact: request.Contact,
	})
	if err != nil {
		return nil, err
	}

	response := toDeskPatientResponse(createdPatient)

	return &response, nil
}

func (dc *DeskController) CreateClaimCode(ctx context.Context, request *desk.CreateClaimCodeRequest) (*desk.ClaimCodeResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	invite, code, err := dc.deskService.WalkInClaimCode(ctx, userID, role, request.ID, models.Relationship(request.Relationship))
	if err != nil {
		return nil, err
	}

	return &desk.ClaimCodeResponse{
		Code:         code,
		Relationship: string(invite.PatientRelationship),
		ExpiresAt:    invite.ExpiresAt,
	}, nil
}

func (dc *DeskController) Book(ctx context.Context, request *desk.BookAtDeskRequest) (*appointment.AppointmentResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	startsAt, err := time.Parse(constant.InstantLayout, request.StartsAt)
	if err != nil {
		return nil, apperrors.Invalid(apperrors.ErrInvalidAppointmentInput, "startsAt", "must be a time with its UTC offset, like 2026-10-26T09:00:00+05:30")
	}

	bookedAppointment, err := dc.appointmentService.BookAtDesk(ctx, userID, role, &models.Appointment{
		PatientID:      request.PatientID,
		StartsAt:       startsAt,
		Mode:           models.ConsultationMode(request.Mode),
		PatientNotes:   request.Notes,
		IdempotencyKey: request.IdempotencyKey,
	})
	if err != nil {
		return nil, err
	}

	response := toAppointmentResponse(bookedAppointment, dc.appointmentService.Location())

	return &response, nil
}

func toDeskPatientResponse(patient *models.Patient) desk.DeskPatientResponse {
	return desk.DeskPatientResponse{
		ID:          patient.ID,
		Name:        patient.Name,
		Gender:      string(patient.Gender),
		DateOfBirth: patient.DateOfBirth.Format(constant.DateLayout),
		BloodType:   string(patient.BloodType),
		Contact:     patient.EmergencyContact,
	}
}
