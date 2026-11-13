package payment

type ListManualRefundsRequest struct {
	Limit  int `form:"limit"`
	Offset int `form:"offset"`
}

// ManualRefundResponse is a refund of a direct UPI payment the doctor owes, with
// the payment and its booking.
type ManualRefundResponse struct {
	Refund      RefundResponse             `json:"refund"`
	Payment     PaymentResponse            `json:"payment"`
	Appointment PaymentAppointmentResponse `json:"appointment"`
}

type ListManualRefundsResponse struct {
	Refunds []ManualRefundResponse `json:"refunds"`
}

// CloseRefundRequest settles a direct UPI refund by hand: refunded when the
// doctor sent the money back, not_received when the payment never arrived (only
// for one that was never verified).
type CloseRefundRequest struct {
	ID      uint64 `json:"-" uri:"id" swaggerignore:"true"`
	Outcome string `json:"outcome" binding:"required,oneof=refunded not_received" enums:"refunded,not_received" example:"refunded"`
	Note    string `json:"note" example:"Sent back by UPI, ref 412398765499"`
}
