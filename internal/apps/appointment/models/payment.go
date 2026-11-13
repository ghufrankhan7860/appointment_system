package models

import "time"

// AppointmentPaymentStatus is the money side of an appointment, kept apart from
// its clinical Status. A free appointment is waived at booking.
type AppointmentPaymentStatus string

const (
	AppointmentPaymentUnpaid   AppointmentPaymentStatus = "unpaid"
	AppointmentPaymentPaid     AppointmentPaymentStatus = "paid"
	AppointmentPaymentRefunded AppointmentPaymentStatus = "refunded"
	AppointmentPaymentWaived   AppointmentPaymentStatus = "waived"
	// PendingVerification: the patient says they paid the doctor by UPI and the
	// doctor has not checked yet. DueAtClinic: the fee is collected at the visit.
	AppointmentPaymentPendingVerification AppointmentPaymentStatus = "pending_verification"
	AppointmentPaymentDueAtClinic         AppointmentPaymentStatus = "due_at_clinic"
)

// PaymentStatus is the state of one attempt to collect an appointment's fee.
type PaymentStatus string

const (
	PaymentStatusCreated  PaymentStatus = "created"
	PaymentStatusCaptured PaymentStatus = "captured"
	PaymentStatusFailed   PaymentStatus = "failed"
	PaymentStatusExpired  PaymentStatus = "expired"
	PaymentStatusRefunded PaymentStatus = "refunded"
	// Claimed: a direct UPI payment the payer says they made; Rejected: the doctor
	// or an admin found it never arrived.
	PaymentStatusClaimed  PaymentStatus = "claimed"
	PaymentStatusRejected PaymentStatus = "rejected"
)

// PaymentMethod is how a patient settles a booking. Which ones exist is set by
// PAYMENT_METHODS; razorpay goes through the gateway, upi straight to the
// doctor, clinic at the visit.
type PaymentMethod string

const (
	PaymentMethodRazorpay PaymentMethod = "razorpay"
	PaymentMethodUPI      PaymentMethod = "upi"
	PaymentMethodClinic   PaymentMethod = "clinic"
)

// The providers of payments that move no money through the gateway: a direct
// UPI payment to the doctor, and a fee collected at the clinic.
const (
	ProviderUPIDirect = "upi_direct"
	ProviderClinic    = "clinic"
)

// PaymentOptions are the methods a booking can be settled with, and the
// doctor's UPI details when upi is one of them.
type PaymentOptions struct {
	Methods []PaymentMethod
	UPI     *DoctorUPI
}

// DoctorUPI is what a patient needs to pay the doctor directly.
type DoctorUPI struct {
	UPIID     string
	PayeeName string
	HasQR     bool
}

type RefundReason string

const (
	RefundReasonPatientCancelled RefundReason = "patient_cancelled"
	RefundReasonDoctorCancelled  RefundReason = "doctor_cancelled"
	RefundReasonLatePayment      RefundReason = "late_payment"
	RefundReasonAdmin            RefundReason = "admin"
)

type RefundStatus string

const (
	RefundStatusRequested RefundStatus = "requested"
	RefundStatusSucceeded RefundStatus = "succeeded"
	RefundStatusFailed    RefundStatus = "failed"
)

// Payment is one attempt to collect an appointment's fee through a gateway.
// Amount is in paise and always comes from the appointment, never the client.
// PayerUserID can differ from the appointment's BookedByUserID.
type Payment struct {
	ID                uint64
	AppointmentID     uint64
	PayerUserID       uint64
	Amount            int
	Currency          string
	Provider          string
	ProviderOrderID   string
	ProviderPaymentID string
	Status            PaymentStatus
	CapturedAt        *time.Time
	// The claim of a direct UPI payment: the payer's UPI reference, their
	// optional screenshot, and who checked it.
	PayerReference   string
	ProofStorageKey  *string
	ClaimedAt        *time.Time
	ReviewedByUserID uint64
	ReviewedAt       *time.Time
	ReviewNote       string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// PaymentStatusChange records one payment status change. ChangedByUserID is 0
// for a change the system made, such as an expiry.
type PaymentStatusChange struct {
	ID              uint64
	PaymentID       uint64
	ChangedByUserID uint64
	FromStatus      PaymentStatus
	ToStatus        PaymentStatus
	Reason          string
	CreatedAt       time.Time
}

// Refund returns a captured payment's full amount. InitiatedByUserID is 0 for
// an automatic refund, such as after a cancellation.
type Refund struct {
	ID                uint64
	PaymentID         uint64
	InitiatedByUserID uint64
	Amount            int
	Reason            RefundReason
	Note              string
	ProviderRefundID  string
	Status            RefundStatus
	ProcessedAt       *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// GatewayEventType names what a gateway callback reports.
type GatewayEventType string

const (
	GatewayEventPaymentCaptured GatewayEventType = "payment_captured"
	GatewayEventRefundProcessed GatewayEventType = "refund_processed"
	GatewayEventRefundFailed    GatewayEventType = "refund_failed"
)

// GatewayEvent is what a gateway's callback says happened. A captured payment
// names its order and gateway payment; a refund names the gateway payment it
// returns, its own id and Amount in paise.
type GatewayEvent struct {
	Type              GatewayEventType
	ProviderOrderID   string
	ProviderPaymentID string
	ProviderRefundID  string
	Amount            int
}
