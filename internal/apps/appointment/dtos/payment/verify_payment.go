package payment

// VerifyPaymentRequest carries the client's proof of payment from the
// gateway's checkout: Razorpay's razorpay_payment_id and razorpay_signature,
// or for the none gateway any payment id and the signature in clientPayload.
type VerifyPaymentRequest struct {
	ID                uint64 `json:"-" uri:"id" swaggerignore:"true"`
	ProviderPaymentID string `json:"providerPaymentId" binding:"required" example:"pay_29QQoUBi66xm2f"`
	Signature         string `json:"signature" binding:"required" example:"9ef4dffbfd84f1318f6739a3ce19f9d85851857ae648f114332d8401e0949a3d"`
}
