package controller

import (
	"appointment_system/internal/apps/appointment/dtos/payment"
	"appointment_system/internal/apps/appointment/models"
)

func toPaymentResponse(paid *models.Payment) payment.PaymentResponse {
	return payment.PaymentResponse{
		ID:                paid.ID,
		AppointmentID:     paid.AppointmentID,
		PayerUserID:       paid.PayerUserID,
		Amount:            paid.Amount,
		Currency:          paid.Currency,
		Provider:          paid.Provider,
		ProviderOrderID:   paid.ProviderOrderID,
		ProviderPaymentID: paid.ProviderPaymentID,
		Status:            string(paid.Status),
		CapturedAt:        paid.CapturedAt,
		CreatedAt:         paid.CreatedAt,
	}
}
