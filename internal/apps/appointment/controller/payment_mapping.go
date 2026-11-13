package controller

import (
	"appointment_system/internal/apps/appointment/dtos/payment"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
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
		PayerReference:    paid.PayerReference,
		HasProof:          paid.ProofStorageKey != nil,
		ClaimedAt:         paid.ClaimedAt,
		ReviewedAt:        paid.ReviewedAt,
		ReviewNote:        paid.ReviewNote,
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

func toPaymentAppointmentResponse(details *models.AppointmentDetails) payment.PaymentAppointmentResponse {
	return payment.PaymentAppointmentResponse{
		ID:            details.ID,
		StartsAt:      details.StartsAt,
		Mode:          string(details.Mode),
		Status:        string(details.Status),
		PaymentStatus: string(details.PaymentStatus),
		PatientName:   details.PatientName,
		DoctorName:    details.DoctorName,
	}
}

func toListClaimsResponse(claims []*services.PaymentClaim) *payment.ListClaimsResponse {
	response := payment.ListClaimsResponse{Claims: make([]payment.ClaimResponse, 0, len(claims))}
	for _, claim := range claims {
		response.Claims = append(response.Claims, payment.ClaimResponse{
			Payment:     toPaymentResponse(claim.Payment),
			Appointment: toPaymentAppointmentResponse(claim.Appointment),
		})
	}

	return &response
}

func toListManualRefundsResponse(refunds []*services.ManualRefund) *payment.ListManualRefundsResponse {
	response := payment.ListManualRefundsResponse{Refunds: make([]payment.ManualRefundResponse, 0, len(refunds))}
	for _, refund := range refunds {
		response.Refunds = append(response.Refunds, payment.ManualRefundResponse{
			Refund:      toRefundResponse(refund.Refund),
			Payment:     toPaymentResponse(refund.Payment),
			Appointment: toPaymentAppointmentResponse(refund.Appointment),
		})
	}

	return &response
}
