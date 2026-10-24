package admin

import "appointment_system/internal/apps/appointment/dtos/payment"

type RefundPaymentRequest struct {
	ID   uint64 `json:"-" uri:"id" swaggerignore:"true"`
	Note string `json:"note" example:"Doctor unavailable, refunded as goodwill"`
}

// RefundPaymentResponse adds whether the refund still waits for an admin to
// pay it back by hand, for a gateway with no refund API.
type RefundPaymentResponse struct {
	payment.PaymentResponse
	RefundPending bool `json:"refundPending"`
}
