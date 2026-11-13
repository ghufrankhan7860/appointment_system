package constant

import "time"

// Rules for collecting appointment fees.
const (
	// PaymentHold is how long an unpaid booking holds its slot before it is
	// cancelled and the slot is freed.
	PaymentHold = 10 * time.Minute
	// PaymentCurrency is the ISO 4217 currency of every fee; amounts are in its
	// minor unit (paise).
	PaymentCurrency = "INR"
	// MaxRefundNoteLength is the longest note on an admin refund.
	MaxRefundNoteLength = 500
	// RefundGatewayGrace is how long a requested refund the gateway has no
	// record of may wait, for a request still in flight, before it is failed.
	RefundGatewayGrace = 15 * time.Minute
	// MaxWebhookBodyBytes caps a gateway callback; real ones are under 2 KB.
	MaxWebhookBodyBytes = 64 << 10
	// MaxPaymentImageBytes caps a direct UPI payment screenshot and a doctor's UPI
	// QR image.
	MaxPaymentImageBytes = 5 << 20
	// MinPayerReferenceLength and MaxPayerReferenceLength bound the UPI reference
	// (UTR) a payer gives; apps show 12-digit references and longer ids.
	MinPayerReferenceLength = 6
	MaxPayerReferenceLength = 35
	// MaxReviewNoteLength is the longest note on checking a claim, collecting a
	// fee at the clinic or closing a manual refund.
	MaxReviewNoteLength = 500
	// MaxUPIIDLength is the longest UPI ID a doctor can set.
	MaxUPIIDLength = 100
	// PaymentProofFormField and UPIQRFormField name the multipart fields of a
	// claim's screenshot and a doctor's QR image.
	PaymentProofFormField = "proof"
	UPIQRFormField        = "file"
)
