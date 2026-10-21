package payment

import "time"

// PaymentResponse is the shape every payment endpoint returns. Amount is in
// paise.
type PaymentResponse struct {
	ID                uint64     `json:"id"`
	AppointmentID     uint64     `json:"appointmentId"`
	PayerUserID       uint64     `json:"payerUserId"`
	Amount            int        `json:"amount" example:"50000"`
	Currency          string     `json:"currency" example:"INR"`
	Provider          string     `json:"provider" example:"none"`
	ProviderOrderID   string     `json:"providerOrderId" example:"none_order_12_1761040000000000000"`
	ProviderPaymentID string     `json:"providerPaymentId,omitempty"`
	Status            string     `json:"status" enums:"created,captured,failed,expired,refunded" example:"created"`
	CapturedAt        *time.Time `json:"capturedAt,omitempty"`
	CreatedAt         time.Time  `json:"createdAt"`
}
