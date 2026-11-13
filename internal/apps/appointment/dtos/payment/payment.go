package payment

import "time"

// PaymentResponse is the shape every payment endpoint returns. Amount is in
// paise. Provider is razorpay (or none locally) for the gateway, upi_direct for
// a direct UPI payment to the doctor and clinic for a fee collected at the
// visit; the claim fields are set on a direct UPI payment the payer says they
// made.
type PaymentResponse struct {
	ID                uint64     `json:"id"`
	AppointmentID     uint64     `json:"appointmentId"`
	PayerUserID       uint64     `json:"payerUserId"`
	Amount            int        `json:"amount" example:"50000"`
	Currency          string     `json:"currency" example:"INR"`
	Provider          string     `json:"provider" enums:"none,razorpay,upi_direct,clinic" example:"upi_direct"`
	ProviderOrderID   string     `json:"providerOrderId" example:"upi_direct_12_a1b2c3d4e5f6"`
	ProviderPaymentID string     `json:"providerPaymentId,omitempty"`
	Status            string     `json:"status" enums:"created,claimed,captured,rejected,failed,expired,refunded" example:"claimed"`
	CapturedAt        *time.Time `json:"capturedAt,omitempty"`
	PayerReference    string     `json:"payerReference,omitempty" example:"412398765432"`
	HasProof          bool       `json:"hasProof"`
	ClaimedAt         *time.Time `json:"claimedAt,omitempty"`
	ReviewedAt        *time.Time `json:"reviewedAt,omitempty"`
	ReviewNote        string     `json:"reviewNote,omitempty"`
	CreatedAt         time.Time  `json:"createdAt"`
}

// PaymentAppointmentResponse is the booking a claim or a refund belongs to,
// enough to find it in the doctor's day.
type PaymentAppointmentResponse struct {
	ID            uint64    `json:"id"`
	StartsAt      time.Time `json:"startsAt"`
	Mode          string    `json:"mode" example:"video"`
	Status        string    `json:"status" example:"scheduled"`
	PaymentStatus string    `json:"paymentStatus" example:"pending_verification"`
	PatientName   string    `json:"patientName"`
	DoctorName    string    `json:"doctorName"`
}
