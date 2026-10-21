package constant

import "time"

// Rules for booking and cancelling appointments.
const (
	// AppointmentCancellationWindow is how long before the start the patient side
	// can still cancel. A doctor can cancel until the start.
	AppointmentCancellationWindow = 2 * time.Hour
	// MaxAppointmentNoteLength is the longest notes or cancel reason text.
	MaxAppointmentNoteLength = 1000
	// MaxIdempotencyKeyLength is the longest Idempotency-Key header accepted.
	MaxIdempotencyKeyLength = 100
	// IdempotencyKeyHeader is the request header that carries the client's key.
	IdempotencyKeyHeader = "Idempotency-Key"

	// ReminderEveningHour is the clinic hour from which the day-before reminder
	// for tomorrow's appointments is sent.
	ReminderEveningHour = 18
	// ReminderLeadTime is how long before the start the last reminder is sent.
	ReminderLeadTime = 2 * time.Hour

	// RescheduleProposalTTL is how long the patient side has to answer a
	// doctor's reschedule proposal. It never runs past the original start.
	RescheduleProposalTTL = 24 * time.Hour
	// MaxProposalNoteLength is the longest note on a reschedule proposal.
	MaxProposalNoteLength = 500
)
