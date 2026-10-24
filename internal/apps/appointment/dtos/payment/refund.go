package payment

import "time"

// RefundResponse is one refund of a payment. Amount is in paise. A requested
// refund waits for an admin to pay it back by hand and complete it.
type RefundResponse struct {
	ID                uint64     `json:"id"`
	PaymentID         uint64     `json:"paymentId"`
	InitiatedByUserID uint64     `json:"initiatedByUserId,omitempty"`
	Amount            int        `json:"amount" example:"50000"`
	Reason            string     `json:"reason" enums:"patient_cancelled,doctor_cancelled,late_payment,admin" example:"doctor_cancelled"`
	Note              string     `json:"note,omitempty"`
	ProviderRefundID  string     `json:"providerRefundId,omitempty" example:"412345678901"`
	Status            string     `json:"status" enums:"requested,succeeded,failed" example:"requested"`
	ProcessedAt       *time.Time `json:"processedAt,omitempty"`
	CreatedAt         time.Time  `json:"createdAt"`
}
