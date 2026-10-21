package payment

type VerifyPaymentRequest struct {
	ID                uint64 `json:"-" uri:"id" swaggerignore:"true"`
	ProviderPaymentID string `json:"providerPaymentId" binding:"required" example:"pay_29QQoUBi66xm2f"`
	Signature         string `json:"signature" binding:"required" example:"none_signature_none_order_12_1761040000000000000"`
}
