package payment

// MarkCollectedRequest records that the doctor collected a booking's fee at the
// clinic; note is optional (cash, UPI at the desk).
type MarkCollectedRequest struct {
	ID   uint64 `json:"-" uri:"id" swaggerignore:"true"`
	Note string `json:"note" example:"Paid in cash"`
}
