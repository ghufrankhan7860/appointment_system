package payment

// VerifyPaymentRequest carries the client's proof of payment. Signature is
// what the gateway's checkout returned; the none gateway needs it, YouPI does
// not because the server asks YouPI itself.
type VerifyPaymentRequest struct {
	ID                uint64 `json:"-" uri:"id" swaggerignore:"true"`
	ProviderPaymentID string `json:"providerPaymentId" binding:"required" example:"pay_29QQoUBi66xm2f"`
	Signature         string `json:"signature" example:"none_signature_none_order_12_1761040000000000000"`
}
