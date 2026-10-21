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
)
