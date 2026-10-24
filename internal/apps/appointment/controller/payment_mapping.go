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

func toRefundResponse(refund *models.Refund) payment.RefundResponse {
	return payment.RefundResponse{
		ID:                refund.ID,
		PaymentID:         refund.PaymentID,
		InitiatedByUserID: refund.InitiatedByUserID,
		Amount:            refund.Amount,
		Reason:            string(refund.Reason),
		Note:              refund.Note,
		ProviderRefundID:  refund.ProviderRefundID,
		Status:            string(refund.Status),
		ProcessedAt:       refund.ProcessedAt,
		CreatedAt:         refund.CreatedAt,
	}
}
