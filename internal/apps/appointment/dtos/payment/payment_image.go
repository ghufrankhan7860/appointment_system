package payment

// GetPaymentImageRequest names the payment or appointment whose screenshot or
// UPI QR is streamed.
type GetPaymentImageRequest struct {
	ID uint64 `uri:"id"`
}
