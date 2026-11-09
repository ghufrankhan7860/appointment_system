// Package models contains appointment domain models and, only when clearly separated,
// persistence records.
//
// Define domain entities and value objects around business meaning and invariants. Keep
// JSON tags, GORM fields, HTTP framework types, and provider wire fields out of domain
// types. If persistence shape differs, create a separately named record and translate at
// the repository boundary.
package models

import "time"

type AppointmentStatus string

const (
	AppointmentStatusScheduled AppointmentStatus = "scheduled"
	AppointmentStatusCompleted AppointmentStatus = "completed"
	AppointmentStatusCancelled AppointmentStatus = "cancelled"
	AppointmentStatusNoShow    AppointmentStatus = "no_show"
	// AppointmentStatusRescheduleProposed marks both the original appointment
	// and the placeholder holding the doctor's offered time while the patient
	// side decides.
	AppointmentStatusRescheduleProposed AppointmentStatus = "reschedule_proposed"
)

type CancelReason string

const (
	CancelReasonPatientRequest   CancelReason = "patient_request"
	CancelReasonDoctorRequest    CancelReason = "doctor_request"
	CancelReasonAdmin            CancelReason = "admin"
	CancelReasonRescheduled      CancelReason = "rescheduled"
	CancelReasonDoctorSuspended  CancelReason = "doctor_suspended"
	CancelReasonPaymentExpired   CancelReason = "payment_expired"
	CancelReasonProposalDeclined CancelReason = "proposal_declined"
	CancelReasonProposalExpired  CancelReason = "proposal_expired"
)

// Appointment is a booked visit of a patient with a doctor. StartsAt and EndsAt
// are instants, not clinic wall-clock minutes. ConsultationFee (paise) and
// SlotDurationMin are copied from the doctor at booking time, so later profile
// changes never alter an existing booking. A rescheduled appointment is a new
// row whose RescheduledFromID points at the cancelled one. IdempotencyKey is
// the client's Idempotency-Key for the request that created it, if any. An
// unpaid appointment holds its slot until PaymentDueAt.
type Appointment struct {
	ID                uint64
	PatientID         uint64
	DoctorID          uint64
	BookedByUserID    uint64
	StartsAt          time.Time
	EndsAt            time.Time
	Mode              ConsultationMode
	Status            AppointmentStatus
	ConsultationFee   int
	SlotDurationMin   int
	PatientNotes      string
	CancelledAt       *time.Time
	CancelledByUserID *uint64
	CancelReason      CancelReason
	RescheduledFromID *uint64
	IdempotencyKey    string
	PaymentStatus     AppointmentPaymentStatus
	PaymentDueAt      *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// AppointmentDetails is an appointment with the names a list screen shows.
// BookedByRelationship is empty when the booker has no link to the patient.
type AppointmentDetails struct {
	Appointment
	DoctorName           string
	DoctorSpeciality     string
	PatientName          string
	BookedByRelationship Relationship
}

// AppointmentStatusChange records one appointment status change and who made it.
// FromStatus is empty for the booking itself; ChangedByUserID is 0 for a change
// the system made, such as an unpaid booking expiring.
type AppointmentStatusChange struct {
	ID              uint64
	AppointmentID   uint64
	ChangedByUserID uint64
	FromStatus      AppointmentStatus
	ToStatus        AppointmentStatus
	Reason          string
	CreatedAt       time.Time
}

// AppointmentFilter narrows a user's appointment list. Zero values mean no filter;
// From is inclusive and To exclusive.
type AppointmentFilter struct {
	PatientID uint64
	Status    AppointmentStatus
	From      time.Time
	To        time.Time
	Limit     int
	Offset    int
}

const OutboxAggregateAppointment = "appointment"

// Events written to the outbox when an appointment changes.
const (
	OutboxEventAppointmentBooked      = "appointment.booked"
	OutboxEventAppointmentCancelled   = "appointment.cancelled"
	OutboxEventAppointmentRescheduled = "appointment.rescheduled"
	OutboxEventAppointmentCompleted   = "appointment.completed"
	OutboxEventAppointmentNoShow      = "appointment.no_show"
	OutboxEventAppointmentReminder    = "appointment.reminder"

	OutboxEventRescheduleProposed = "appointment.reschedule_proposed"
	OutboxEventRescheduleDeclined = "appointment.reschedule_declined"
	OutboxEventRescheduleExpired  = "appointment.reschedule_expired"

	OutboxAggregateMedicalRecord      = "medical_record"
	OutboxEventMedicalRecordFinalized = "medical_record.finalized"

	OutboxAggregatePayment     = "payment"
	OutboxEventPaymentCaptured = "payment.captured"
	OutboxEventPaymentRefunded = "payment.refunded"
	// OutboxEventRefundFailed tells admins a refund did not go through, so
	// they can retry it.
	OutboxEventRefundFailed = "refund.failed"

	OutboxAggregatePatient          = "patient"
	OutboxEventPatientAccessGranted = "patient.access_granted"
	OutboxEventPatientAccessChanged = "patient.access_changed"
	OutboxEventPatientAccessRevoked = "patient.access_revoked"

	OutboxAggregateCall    = "call"
	OutboxEventCallStarted = "call.started"
	OutboxEventCallEnded   = "call.ended"
)

// ReminderWindow names one reminder sent before an appointment. Each window is
// sent at most once per appointment.
type ReminderWindow string

const (
	ReminderWindowDayBefore ReminderWindow = "day_before"
	ReminderWindowTwoHours  ReminderWindow = "two_hours"
)

// OutboxEvent is a change to publish later, written in the same transaction as
// the change itself so it is never lost or sent for a rolled-back change.
type OutboxEvent struct {
	ID            uint64
	AggregateType string
	AggregateID   uint64
	EventType     string
	Payload       map[string]any
	CreatedAt     time.Time
	PublishedAt   *time.Time
}

// DoctorPatient is one patient in a doctor's patient list, built from the
// bookings that count: completed, no-show, and scheduled ones that are not an
// unpaid hold. Visits counts those bookings.
type DoctorPatient struct {
	PatientID         uint64
	Name              string
	Gender            Gender
	DateOfBirth       time.Time
	LastAppointmentAt *time.Time
	NextAppointmentAt *time.Time
	Visits            int
}
