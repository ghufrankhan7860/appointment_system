package payment

type GetPaymentOptionsRequest struct {
	ID uint64 `uri:"id"`
}

// UPIDetailsResponse is what a patient needs to pay the doctor directly.
type UPIDetailsResponse struct {
	UPIID     string `json:"upiId" example:"drasha@okhdfcbank"`
	PayeeName string `json:"payeeName" example:"Dr. Asha Verma"`
	HasQR     bool   `json:"hasQr"`
}

// PaymentOptionsResponse lists how the booking can be paid; upi is set when it
// is one of the methods.
type PaymentOptionsResponse struct {
	Methods []string            `json:"methods" enums:"razorpay,upi,clinic" example:"upi,clinic"`
	UPI     *UPIDetailsResponse `json:"upi,omitempty"`
}
