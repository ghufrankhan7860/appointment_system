package controller

import (
	"context"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/dtos/auth"
	"appointment_system/internal/apps/appointment/dtos/staff"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
)

type StaffController struct {
	staffService *services.StaffService
}

func NewStaffController(staffService *services.StaffService) (*StaffController, error) {
	if staffService == nil {
		return nil, apperrors.ErrNilStaffService
	}

	return &StaffController{staffService: staffService}, nil
}

func (sc *StaffController) Register(ctx context.Context, request *staff.RegisterStaffRequest, clientIP string) (*auth.RegisterResponse, error) {
	user := models.User{
		Name:     request.Name,
		Email:    request.Email,
		MobileNo: request.MobileNo,
	}
	createdUser, err := sc.staffService.Register(ctx, &user, request.Password, request.InviteCode, clientIP)
	if err != nil {
		return nil, err
	}

	return &auth.RegisterResponse{
		ID:       createdUser.ID,
		Name:     createdUser.Name,
		Email:    createdUser.Email,
		MobileNo: createdUser.MobileNo,
		Role:     string(createdUser.Role),
	}, nil
}

func (sc *StaffController) Me(ctx context.Context) (*staff.GetStaffMeResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	link, doctor, err := sc.staffService.Me(ctx, userID, role)
	if err != nil {
		return nil, err
	}

	return &staff.GetStaffMeResponse{JoinedAt: link.CreatedAt, Doctor: toDoctorResponse(doctor)}, nil
}

func (sc *StaffController) Join(ctx context.Context, request *staff.JoinStaffRequest, clientIP string) (*staff.GetStaffMeResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	if _, err := sc.staffService.Join(ctx, userID, role, request.InviteCode, clientIP); err != nil {
		return nil, err
	}

	return sc.Me(ctx)
}

func (sc *StaffController) CreateInvite(ctx context.Context) (*staff.StaffInviteResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	createdInvite, code, err := sc.staffService.CreateInvite(ctx, userID, role)
	if err != nil {
		return nil, err
	}

	response := toStaffInviteResponse(createdInvite, code)

	return &response, nil
}

func (sc *StaffController) ListInvites(ctx context.Context) (*staff.ListStaffInvitesResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	invites, err := sc.staffService.ListInvites(ctx, userID, role)
	if err != nil {
		return nil, err
	}

	response := staff.ListStaffInvitesResponse{Invites: make([]staff.StaffInviteResponse, 0, len(invites))}
	for _, invite := range invites {
		response.Invites = append(response.Invites, toStaffInviteResponse(invite, ""))
	}

	return &response, nil
}

func (sc *StaffController) RevokeInvite(ctx context.Context, request *staff.RevokeStaffInviteRequest) error {
	userID, err := actingUserID(ctx)
	if err != nil {
		return err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return err
	}

	return sc.staffService.RevokeInvite(ctx, userID, role, request.ID)
}

func (sc *StaffController) List(ctx context.Context) (*staff.ListStaffResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	members, err := sc.staffService.List(ctx, userID, role)
	if err != nil {
		return nil, err
	}

	response := staff.ListStaffResponse{Staff: make([]staff.StaffMemberResponse, 0, len(members))}
	for _, member := range members {
		response.Staff = append(response.Staff, staff.StaffMemberResponse{
			UserID:   member.UserID,
			Name:     member.Name,
			Email:    member.Email,
			MobileNo: member.MobileNo,
			JoinedAt: member.CreatedAt,
		})
	}

	return &response, nil
}

func (sc *StaffController) Remove(ctx context.Context, request *staff.RemoveStaffRequest) error {
	userID, err := actingUserID(ctx)
	if err != nil {
		return err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return err
	}

	return sc.staffService.Remove(ctx, userID, role, request.UserID)
}

func toStaffInviteResponse(invite *models.StaffInvite, code string) staff.StaffInviteResponse {
	return staff.StaffInviteResponse{
		ID:        invite.ID,
		Code:      code,
		ExpiresAt: invite.ExpiresAt,
		CreatedAt: invite.CreatedAt,
	}
}
