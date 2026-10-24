package controller

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/dtos/admin"
	"appointment_system/internal/apps/appointment/dtos/payment"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
	"context"
)

type AdminController struct {
	adminUserService          *services.AdminUserService
	doctorVerificationService *services.DoctorVerificationService
	doctorService             *services.DoctorService
	paymentService            *services.PaymentService
	medicalRecordService      *services.MedicalRecordService
}

func NewAdminController(
	adminUserService *services.AdminUserService,
	doctorVerificationService *services.DoctorVerificationService,
	doctorService *services.DoctorService,
	paymentService *services.PaymentService,
	medicalRecordService *services.MedicalRecordService,
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
	if paymentService == nil {
		return nil, apperrors.ErrNilPaymentService
	}
	if medicalRecordService == nil {
		return nil, apperrors.ErrNilMedicalRecordService
	}
	return &AdminController{
		adminUserService:          adminUserService,
		doctorVerificationService: doctorVerificationService,
		doctorService:             doctorService,
		paymentService:            paymentService,
		medicalRecordService:      medicalRecordService,
	}, nil
}

func (ac *AdminController) ListRecordAccessLog(ctx context.Context, request *admin.ListAccessLogRequest) (*admin.ListAccessLogResponse, error) {
	entries, err := ac.medicalRecordService.ListAccessLog(ctx, request.ID, request.Limit, request.Offset)
	if err != nil {
		return nil, err
	}

	response := admin.ListAccessLogResponse{
		Entries: make([]admin.RecordAccessResponse, 0, len(entries)),
	}
	for _, entry := range entries {
		response.Entries = append(response.Entries, admin.RecordAccessResponse{
			ID:               entry.ID,
			PatientID:        entry.PatientID,
			MedicalRecordID:  entry.MedicalRecordID,
			AttachmentID:     entry.AttachmentID,
			AccessedByUserID: entry.AccessedByUserID,
			Action:           string(entry.Action),
			CorrelationID:    entry.CorrelationID,
			CreatedAt:        entry.CreatedAt,
		})
	}

	return &response, nil
}

func (ac *AdminController) RefundPayment(ctx context.Context, request *admin.RefundPaymentRequest) (*admin.RefundPaymentResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}

	refundedPayment, err := ac.paymentService.Refund(ctx, userID, request.ID, request.Note)
	if err != nil {
		return nil, err
	}

	return &admin.RefundPaymentResponse{
		PaymentResponse: toPaymentResponse(refundedPayment),
		RefundPending:   refundedPayment.Status != models.PaymentStatusRefunded,
	}, nil
}

func (ac *AdminController) ListRefunds(ctx context.Context, request *admin.ListRefundsRequest) (*admin.ListRefundsResponse, error) {
	refunds, err := ac.paymentService.ListRefunds(ctx, models.RefundStatus(request.Status), request.Limit, request.Offset)
	if err != nil {
		return nil, err
	}

	response := admin.ListRefundsResponse{
		Refunds: make([]payment.RefundResponse, 0, len(refunds)),
	}
	for _, refund := range refunds {
		response.Refunds = append(response.Refunds, toRefundResponse(refund))
	}

	return &response, nil
}

func (ac *AdminController) CompleteRefund(ctx context.Context, request *admin.CompleteRefundRequest) (*payment.RefundResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}

	completedRefund, err := ac.paymentService.CompleteRefund(ctx, userID, request.ID, request.Reference)
	if err != nil {
		return nil, err
	}

	response := toRefundResponse(completedRefund)
	return &response, nil
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
