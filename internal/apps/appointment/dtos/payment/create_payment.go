package payment

type CreatePaymentRequest struct {
	ID uint64 `uri:"id"`
}

// CreatePaymentResponse adds what the client needs to open the gateway's
// payment screen. With the none gateway it carries the signature to verify with.
type CreatePaymentResponse struct {
	PaymentResponse
	ClientPayload map[string]any `json:"clientPayload"`
}
