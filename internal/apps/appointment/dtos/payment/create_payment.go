package payment

// CreatePaymentRequest picks how to pay: razorpay (the default) opens a gateway
// order, upi a direct UPI payment to the doctor.
type CreatePaymentRequest struct {
	ID     uint64 `json:"-" uri:"id" swaggerignore:"true"`
	Method string `json:"method" binding:"omitempty,oneof=razorpay upi" enums:"razorpay,upi" example:"upi"`
}

// CreatePaymentResponse adds what the client needs to pay. For razorpay it opens
// the gateway's payment screen (with the none gateway it carries the signature to
// verify with); for upi it has upiId, payeeName, amount, note, upiLink and hasQr.
type CreatePaymentResponse struct {
	PaymentResponse
	ClientPayload map[string]any `json:"clientPayload"`
}
