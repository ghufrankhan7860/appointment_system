package payment

// ClaimPaymentRequest carries the form fields of the multipart claim: the UPI
// reference (UTR) from the payer's UPI app. The optional screenshot is the
// "proof" file field. ID is the payment, from the path.
type ClaimPaymentRequest struct {
	ID        uint64 `form:"-" uri:"id" swaggerignore:"true"`
	Reference string `form:"reference" binding:"required"`
}
