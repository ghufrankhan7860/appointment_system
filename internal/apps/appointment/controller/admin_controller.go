package controller

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/dtos/admin"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
	"context"
)

type AdminController struct {
	adminUserService          *services.AdminUserService
	doctorVerificationService *services.DoctorVerificationService
	doctorService             *services.DoctorService
}

func NewAdminController(
	adminUserService *services.AdminUserService,
	doctorVerificationService *services.DoctorVerificationService,
	doctorService *services.DoctorService,
) (*AdminController, error) {
	if adminUserService == nil {
		return nil, apperrors.ErrNilAdminUserService
	}
	if doctorVerificationService == nil {
		return nil, apperrors.ErrNilDoctorVerificationService
	}
	if doctorService == nil {
		return nil, apperrors.ErrNilDoctorService
	}
	return &AdminController{
		adminUserService:          adminUserService,
		doctorVerificationService: doctorVerificationService,
		doctorService:             doctorService,
	}, nil
}

func (ac *AdminController) CreateAdmin(ctx context.Context, request *admin.CreateAdminRequest) (*admin.CreateAdminResponse, error) {
	user := models.User{
		Name:     request.Name,
		Email:    request.Email,
		MobileNo: request.MobileNo,
	}
	createdUser, err := ac.adminUserService.Create(ctx, &user, request.Password)
	if err != nil {
		return nil, err
	}

	return &admin.CreateAdminResponse{
		ID:       createdUser.ID,
		Name:     createdUser.Name,
		Email:    createdUser.Email,
		MobileNo: createdUser.MobileNo,
		Role:     string(createdUser.Role),
	}, nil
}

func (ac *AdminController) ListDoctors(ctx context.Context, request *admin.ListDoctorsRequest) (*admin.ListDoctorsResponse, error) {
	doctors, err := ac.doctorVerificationService.List(ctx, models.VerificationStatus(request.Status), request.Limit, request.Offset)
	if err != nil {
		return nil, err
	}

	response := admin.ListDoctorsResponse{
		Doctors: make([]admin.GetDoctorResponse, 0, len(doctors)),
	}
	for _, listedDoctor := range doctors {
		response.Doctors = append(response.Doctors, admin.GetDoctorResponse{
			DoctorResponse: toDoctorResponse(listedDoctor),
			UserID:         listedDoctor.UserID,
		})
	}

	return &response, nil
}

func (ac *AdminController) GetDoctor(ctx context.Context, request *admin.GetDoctorRequest) (*admin.GetDoctorResponse, error) {
	foundDoctor, verifications, err := ac.doctorVerificationService.GetByID(ctx, request.ID)
	if err != nil {
		return nil, err
	}

	response := admin.GetDoctorResponse{
		DoctorResponse: toDoctorResponse(foundDoctor),
		UserID:         foundDoctor.UserID,
		Verifications:  make([]admin.VerificationResponse, 0, len(verifications)),
	}
	for _, verification := range verifications {
		response.Verifications = append(response.Verifications, admin.VerificationResponse{
			ReviewedByUserID: verification.ReviewedByUserID,
			FromStatus:       string(verification.FromStatus),
			ToStatus:         string(verification.ToStatus),
			Reason:           verification.Reason,
			CreatedAt:        verification.CreatedAt,
		})
	}

	return &response, nil
}

func (ac *AdminController) VerifyDoctor(ctx context.Context, request *admin.ReviewDoctorRequest) (*admin.GetDoctorResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}

	reviewedDoctor, err := ac.doctorVerificationService.Verify(ctx, userID, request.ID)
	if err != nil {
		return nil, err
	}

	return &admin.GetDoctorResponse{
		DoctorResponse: toDoctorResponse(reviewedDoctor),
		UserID:         reviewedDoctor.UserID,
	}, nil
}

func (ac *AdminController) RejectDoctor(ctx context.Context, request *admin.ReviewDoctorRequest) (*admin.GetDoctorResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}

	reviewedDoctor, err := ac.doctorVerificationService.Reject(ctx, userID, request.ID, request.Reason)
	if err != nil {
		return nil, err
	}

	return &admin.GetDoctorResponse{
		DoctorResponse: toDoctorResponse(reviewedDoctor),
		UserID:         reviewedDoctor.UserID,
	}, nil
}

func (ac *AdminController) SuspendDoctor(ctx context.Context, request *admin.ReviewDoctorRequest) (*admin.GetDoctorResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}

	reviewedDoctor, err := ac.doctorVerificationService.Suspend(ctx, userID, request.ID, request.Reason)
	if err != nil {
		return nil, err
	}

	return &admin.GetDoctorResponse{
		DoctorResponse: toDoctorResponse(reviewedDoctor),
		UserID:         reviewedDoctor.UserID,
	}, nil
}

func (ac *AdminController) ReinstateDoctor(ctx context.Context, request *admin.ReviewDoctorRequest) (*admin.GetDoctorResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}

	reviewedDoctor, err := ac.doctorVerificationService.Reinstate(ctx, userID, request.ID, request.Reason)
	if err != nil {
		return nil, err
	}

	return &admin.GetDoctorResponse{
		DoctorResponse: toDoctorResponse(reviewedDoctor),
		UserID:         reviewedDoctor.UserID,
	}, nil
}

func (ac *AdminController) CreateSpeciality(ctx context.Context, request *admin.CreateSpecialityRequest) (*admin.CreateSpecialityResponse, error) {
	createdSpeciality, err := ac.doctorService.CreateSpeciality(ctx, request.Slug, request.Name)
	if err != nil {
		return nil, err
	}

	return &admin.CreateSpecialityResponse{
		ID:   createdSpeciality.ID,
		Slug: createdSpeciality.Slug,
		Name: createdSpeciality.Name,
	}, nil
}
