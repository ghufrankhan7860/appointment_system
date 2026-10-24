package controller

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/dtos/payment"
	"appointment_system/internal/apps/appointment/services"
	"context"
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

	createdPayment, clientPayload, err := pc.paymentService.Create(ctx, userID, request.ID)
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
