package doctor

// UpdateUPIRequest sets the UPI ID patients pay the doctor at; empty stops
// direct UPI payments (and removes the QR).
type UpdateUPIRequest struct {
	UPIID string `json:"upiId" example:"drasha@okhdfcbank"`
}
