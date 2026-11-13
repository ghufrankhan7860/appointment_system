package payment

// VerifyClaimRequest confirms a direct UPI payment arrived; note is optional.
type VerifyClaimRequest struct {
	ID   uint64 `json:"-" uri:"id" swaggerignore:"true"`
	Note string `json:"note" example:"Seen in my bank app"`
}

// RejectClaimRequest records that a direct UPI payment never arrived; the
// reason is shown to the patient.
type RejectClaimRequest struct {
	ID     uint64 `json:"-" uri:"id" swaggerignore:"true"`
	Reason string `json:"reason" binding:"required" example:"No payment with this reference reached my account"`
}

type ListClaimsRequest struct {
	Limit  int `form:"limit"`
	Offset int `form:"offset"`
}

// ClaimResponse is a direct UPI payment waiting for a check, with its booking.
type ClaimResponse struct {
	Payment     PaymentResponse            `json:"payment"`
	Appointment PaymentAppointmentResponse `json:"appointment"`
}

type ListClaimsResponse struct {
	Claims []ClaimResponse `json:"claims"`
}
