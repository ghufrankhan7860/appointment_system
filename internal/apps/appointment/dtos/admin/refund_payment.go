package admin

import "appointment_system/internal/apps/appointment/dtos/payment"

type RefundPaymentRequest struct {
	ID   uint64 `json:"-" uri:"id" swaggerignore:"true"`
	Note string `json:"note" example:"Doctor unavailable, refunded as goodwill"`
}

// RefundPaymentResponse adds whether the gateway is still processing the
// refund; the payment stays captured until it finishes.
type RefundPaymentResponse struct {
	payment.PaymentResponse
	RefundPending bool `json:"refundPending"`
}
