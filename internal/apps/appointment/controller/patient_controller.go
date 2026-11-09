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
	patientService    *services.PatientService
	inviteService     *services.PatientInviteService
	membershipService *services.PatientMembershipService
}

func NewPatientController(
	patientService *services.PatientService,
	inviteService *services.PatientInviteService,
	membershipService *services.PatientMembershipService,
) (*PatientController, error) {
	if patientService == nil {
		return nil, apperrors.ErrNilPatientService
	}
	if inviteService == nil {
		return nil, apperrors.ErrNilPatientInviteService
	}
	if membershipService == nil {
		return nil, apperrors.ErrNilPatientMembershipService
	}
	return &PatientController{
		patientService:    patientService,
		inviteService:     inviteService,
		membershipService: membershipService,
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

	response := toPatientResponse(foundPatient)

	return &response, nil
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
		response.Patients = append(response.Patients, toPatientResponse(listedPatient))
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

func (pc *PatientController) CreateInvite(ctx context.Context, request *patient.CreatePatientInviteRequest) (*patient.PatientInviteResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}

	expiresIn := time.Duration(request.ExpiresInHours) * time.Hour
	createdInvite, code, err := pc.inviteService.Create(ctx, userID, request.ID, models.Relationship(request.Relationship), models.AccessLevel(request.AccessLevel), expiresIn)
	if err != nil {
		return nil, err
	}

	response := toPatientInviteResponse(createdInvite, code)

	return &response, nil
}

func (pc *PatientController) ListInvites(ctx context.Context, request *patient.ListPatientInvitesRequest) (*patient.ListPatientInvitesResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}

	invites, err := pc.inviteService.List(ctx, userID, request.ID)
	if err != nil {
		return nil, err
	}

	response := patient.ListPatientInvitesResponse{
		Invites: make([]patient.PatientInviteResponse, 0, len(invites)),
	}
	for _, invite := range invites {
		response.Invites = append(response.Invites, toPatientInviteResponse(invite, ""))
	}

	return &response, nil
}

func (pc *PatientController) RevokeInvite(ctx context.Context, request *patient.RevokePatientInviteRequest) error {
	userID, err := actingUserID(ctx)
	if err != nil {
		return err
	}

	return pc.inviteService.Revoke(ctx, userID, request.ID, request.InviteID)
}

func (pc *PatientController) Claim(ctx context.Context, request *patient.ClaimPatientRequest) (*patient.GetPatientResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}

	claimedPatient, err := pc.inviteService.Redeem(ctx, userID, request.Code)
	if err != nil {
		return nil, err
	}

	response := toPatientResponse(claimedPatient)

	return &response, nil
}

func (pc *PatientController) ListMembers(ctx context.Context, request *patient.ListPatientMembersRequest) (*patient.ListPatientMembersResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}

	members, err := pc.membershipService.List(ctx, userID, request.ID)
	if err != nil {
		return nil, err
	}

	response := patient.ListPatientMembersResponse{
		Members: make([]patient.PatientMemberResponse, 0, len(members)),
	}
	for _, member := range members {
		response.Members = append(response.Members, patient.PatientMemberResponse{
			UserID:       member.Link.UserID,
			Name:         member.Name,
			MaskedMobile: maskMobile(member.MobileNo),
			Relationship: string(member.Link.PatientRelationship),
			AccessLevel:  string(member.Link.AccessLevel),
			Current:      member.Link.UserID == userID,
			Since:        member.Link.UpdatedAt.UTC(),
		})
	}

	return &response, nil
}

func (pc *PatientController) UpdateMember(ctx context.Context, request *patient.UpdatePatientMemberRequest) error {
	userID, err := actingUserID(ctx)
	if err != nil {
		return err
	}

	_, err = pc.membershipService.Update(ctx, userID, request.ID, request.UserID, models.AccessLevel(request.AccessLevel))

	return err
}

func (pc *PatientController) RevokeMember(ctx context.Context, request *patient.RevokePatientMemberRequest) error {
	userID, err := actingUserID(ctx)
	if err != nil {
		return err
	}

	return pc.membershipService.Revoke(ctx, userID, request.ID, request.UserID)
}

func (pc *PatientController) Leave(ctx context.Context, request *patient.LeavePatientRequest) error {
	userID, err := actingUserID(ctx)
	if err != nil {
		return err
	}

	return pc.membershipService.Leave(ctx, userID, request.ID)
}
