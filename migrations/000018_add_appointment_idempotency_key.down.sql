DROP INDEX IF EXISTS idx_appointments_booked_by_user_id_idempotency_key_unique;

ALTER TABLE appointments
DROP COLUMN IF EXISTS idempotency_key;
