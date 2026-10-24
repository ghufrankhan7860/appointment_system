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
	// MaxRefundReferenceLength is the longest payout reference, such as a UPI
	// UTR, an admin records for a manual refund.
	MaxRefundReferenceLength = 100
	// MaxWebhookBodyBytes caps a gateway callback; real ones are under 2 KB.
	MaxWebhookBodyBytes = 64 << 10
)
