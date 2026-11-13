package controller

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/dtos/doctor"
	"appointment_system/internal/apps/appointment/dtos/payment"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
	"context"
	"io"
)

type PaymentController struct {
	paymentService *services.PaymentService
}

func NewPaymentController(paymentService *services.PaymentService) (*PaymentController, error) {
	if paymentService == nil {
		return nil, apperrors.ErrNilPaymentService
	}
	return &PaymentController{
		paymentService: paymentService,
	}, nil
}

func (pc *PaymentController) Create(ctx context.Context, request *payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}

	create := pc.paymentService.Create
	if request.Method == string(models.PaymentMethodUPI) {
		create = pc.paymentService.StartUPI
	}
	createdPayment, clientPayload, err := create(ctx, userID, request.ID)
	if err != nil {
		return nil, err
	}

	return &payment.CreatePaymentResponse{
		PaymentResponse: toPaymentResponse(createdPayment),
		ClientPayload:   clientPayload,
	}, nil
}

func (pc *PaymentController) GetForAppointment(ctx context.Context, request *payment.GetAppointmentPaymentRequest) (*payment.PaymentResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	foundPayment, err := pc.paymentService.GetForAppointment(ctx, userID, role, request.ID)
	if err != nil {
		return nil, err
	}

	response := toPaymentResponse(foundPayment)
	return &response, nil
}

func (pc *PaymentController) Verify(ctx context.Context, request *payment.VerifyPaymentRequest) (*payment.PaymentResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}

	verifiedPayment, err := pc.paymentService.Verify(ctx, userID, request.ID, request.ProviderPaymentID, request.Signature)
	if err != nil {
		return nil, err
	}

	response := toPaymentResponse(verifiedPayment)
	return &response, nil
}

// HandleWebhook passes a gateway callback's raw body and headers through; the
// gateway's signature covers the exact bytes, so nothing is decoded here.
func (pc *PaymentController) HandleWebhook(ctx context.Context, body []byte, headers map[string][]string) error {
	return pc.paymentService.HandleWebhook(ctx, body, headers)
}

func (pc *PaymentController) Options(ctx context.Context, request *payment.GetPaymentOptionsRequest) (*payment.PaymentOptionsResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}

	options, err := pc.paymentService.Options(ctx, userID, request.ID)
	if err != nil {
		return nil, err
	}

	response := payment.PaymentOptionsResponse{Methods: make([]string, 0, len(options.Methods))}
	for _, method := range options.Methods {
		response.Methods = append(response.Methods, string(method))
	}
	if options.UPI != nil {
		response.UPI = &payment.UPIDetailsResponse{
			UPIID:     options.UPI.UPIID,
			PayeeName: options.UPI.PayeeName,
			HasQR:     options.UPI.HasQR,
		}
	}

	return &response, nil
}

func (pc *PaymentController) PayAtClinic(ctx context.Context, request *payment.PayAtClinicRequest) error {
	userID, err := actingUserID(ctx)
	if err != nil {
		return err
	}

	return pc.paymentService.PayAtClinic(ctx, userID, request.ID)
}

func (pc *PaymentController) Claim(ctx context.Context, request *payment.ClaimPaymentRequest, proof io.Reader) (*payment.PaymentResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}

	claimedPayment, err := pc.paymentService.Claim(ctx, userID, request.ID, request.Reference, proof)
	if err != nil {
		return nil, err
	}

	response := toPaymentResponse(claimedPayment)

	return &response, nil
}

func (pc *PaymentController) OpenProof(ctx context.Context, request *payment.GetPaymentImageRequest) (io.ReadCloser, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	return pc.paymentService.OpenProof(ctx, userID, role, request.ID)
}

// OpenUPIQR streams the QR of the appointment's doctor, or the acting doctor's
// own when request is nil.
func (pc *PaymentController) OpenUPIQR(ctx context.Context, request *payment.GetPaymentImageRequest) (io.ReadCloser, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	appointmentID := uint64(0)
	if request != nil {
		appointmentID = request.ID
	}

	return pc.paymentService.OpenUPIQR(ctx, userID, role, appointmentID)
}

func (pc *PaymentController) VerifyClaim(ctx context.Context, request *payment.VerifyClaimRequest) (*payment.PaymentResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	verifiedPayment, err := pc.paymentService.VerifyClaim(ctx, userID, role, request.ID, request.Note)
	if err != nil {
		return nil, err
	}

	response := toPaymentResponse(verifiedPayment)

	return &response, nil
}

func (pc *PaymentController) RejectClaim(ctx context.Context, request *payment.RejectClaimRequest) (*payment.PaymentResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	rejectedPayment, err := pc.paymentService.RejectClaim(ctx, userID, role, request.ID, request.Reason)
	if err != nil {
		return nil, err
	}

	response := toPaymentResponse(rejectedPayment)

	return &response, nil
}

func (pc *PaymentController) ListClaims(ctx context.Context, request *payment.ListClaimsRequest) (*payment.ListClaimsResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	claims, err := pc.paymentService.ListClaims(ctx, userID, role, request.Limit, request.Offset)
	if err != nil {
		return nil, err
	}

	return toListClaimsResponse(claims), nil
}

func (pc *PaymentController) MarkCollected(ctx context.Context, request *payment.MarkCollectedRequest) (*payment.PaymentResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}

	collectedPayment, err := pc.paymentService.MarkCollected(ctx, userID, request.ID, request.Note)
	if err != nil {
		return nil, err
	}

	response := toPaymentResponse(collectedPayment)

	return &response, nil
}

func (pc *PaymentController) ListManualRefunds(ctx context.Context, request *payment.ListManualRefundsRequest) (*payment.ListManualRefundsResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	refunds, err := pc.paymentService.ListManualRefunds(ctx, userID, role, request.Limit, request.Offset)
	if err != nil {
		return nil, err
	}

	return toListManualRefundsResponse(refunds), nil
}

func (pc *PaymentController) CloseRefund(ctx context.Context, request *payment.CloseRefundRequest) error {
	userID, err := actingUserID(ctx)
	if err != nil {
		return err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return err
	}

	return pc.paymentService.CloseManualRefund(ctx, userID, role, request.ID, request.Outcome == "refunded", request.Note)
}

func (pc *PaymentController) UpdateUPI(ctx context.Context, request *doctor.UpdateUPIRequest) (*doctor.DoctorResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	updatedDoctor, err := pc.paymentService.SetDoctorUPI(ctx, userID, role, request.UPIID)
	if err != nil {
		return nil, err
	}

	response := toDoctorResponse(updatedDoctor)

	return &response, nil
}

func (pc *PaymentController) SetUPIQR(ctx context.Context, content io.Reader) (*doctor.DoctorResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	updatedDoctor, err := pc.paymentService.SetDoctorUPIQR(ctx, userID, role, content)
	if err != nil {
		return nil, err
	}

	response := toDoctorResponse(updatedDoctor)

	return &response, nil
}

func (pc *PaymentController) DeleteUPIQR(ctx context.Context) (*doctor.DoctorResponse, error) {
	userID, err := actingUserID(ctx)
	if err != nil {
		return nil, err
	}
	role, err := actingRole(ctx)
	if err != nil {
		return nil, err
	}

	updatedDoctor, err := pc.paymentService.DeleteDoctorUPIQR(ctx, userID, role)
	if err != nil {
		return nil, err
	}

	response := toDoctorResponse(updatedDoctor)

	return &response, nil
}
