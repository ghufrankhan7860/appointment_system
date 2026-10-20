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
)

func (constraint uniqueConstraint) ViolatedBy(err error) bool {
	var pgError *pgconn.PgError

	return errors.As(err, &pgError) &&
		pgError.Code == uniqueViolationCode &&
		pgError.ConstraintName == string(constraint)
}
