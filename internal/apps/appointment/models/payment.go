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
)

// PaymentStatus is the state of one attempt to collect an appointment's fee.
type PaymentStatus string

const (
	PaymentStatusCreated  PaymentStatus = "created"
	PaymentStatusCaptured PaymentStatus = "captured"
	PaymentStatusFailed   PaymentStatus = "failed"
	PaymentStatusExpired  PaymentStatus = "expired"
	PaymentStatusRefunded PaymentStatus = "refunded"
)

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

// GatewayEvent is what a gateway's callback says happened to one of its
// orders. Status is captured, failed or expired; Amount is in paise.
type GatewayEvent struct {
	ProviderOrderID   string
	ProviderPaymentID string
	Amount            int
	Status            PaymentStatus
}
