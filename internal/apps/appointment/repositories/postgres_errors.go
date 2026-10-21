package repositories

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

const uniqueViolationCode = "23505"

type uniqueConstraint string

// The constraint names Postgres generated from the migrations. A migration that
// renames one must update it here.
const (
	emailUnique           uniqueConstraint = "users_email_key"
	mobileUnique          uniqueConstraint = "users_mobile_no_key"
	tokenHashUnique       uniqueConstraint = "sessions_token_hash_key"
	userPatientLinkUnique uniqueConstraint = "user_patient_pkey"
	doctorProfileUnique   uniqueConstraint = "doctors_user_id_key"
	registrationUnique    uniqueConstraint = "doctors_registration_council_registration_number_key"
	specialitySlugUnique  uniqueConstraint = "specialities_slug_key"

	appointmentIdempotencyKeyUnique uniqueConstraint = "idx_appointments_booked_by_user_id_idempotency_key_unique"
	livePaymentUnique               uniqueConstraint = "idx_payments_appointment_id_live_unique"
	paymentRefundUnique             uniqueConstraint = "idx_refunds_payment_id_unique"
	providerPaymentUnique           uniqueConstraint = "idx_payments_provider_provider_payment_id_unique"
	pendingProposalUnique           uniqueConstraint = "idx_reschedule_proposals_appointment_id_pending_unique"
	medicalRecordAppointmentUnique  uniqueConstraint = "medical_records_appointment_id_key"
)

func (constraint uniqueConstraint) ViolatedBy(err error) bool {
	var pgError *pgconn.PgError

	return errors.As(err, &pgError) &&
		pgError.Code == uniqueViolationCode &&
		pgError.ConstraintName == string(constraint)
}

const exclusionViolationCode = "23P01"

type exclusionConstraint string

// The EXCLUDE constraints named in the migrations.
const (
	doctorNoOverlapExclusion exclusionConstraint = "appointments_doctor_no_overlap"
)

func (constraint exclusionConstraint) ViolatedBy(err error) bool {
	var pgError *pgconn.PgError

	return errors.As(err, &pgError) &&
		pgError.Code == exclusionViolationCode &&
		pgError.ConstraintName == string(constraint)
}
