package appointment

// CloseAppointmentRequest identifies the appointment a doctor completes or
// marks as a no-show.
type CloseAppointmentRequest struct {
	ID uint64 `uri:"id"`
}
