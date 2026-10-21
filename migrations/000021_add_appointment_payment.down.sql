ALTER TABLE appointment_status_history
ALTER COLUMN changed_by_user_id SET NOT NULL;

DROP INDEX IF EXISTS idx_appointments_payment_due_at;

ALTER TABLE appointments
DROP CONSTRAINT IF EXISTS appointments_cancel_reason_check;

ALTER TABLE appointments
ADD CONSTRAINT appointments_cancel_reason_check CHECK (cancel_reason IN ('patient_request', 'doctor_request', 'admin', 'rescheduled', 'doctor_suspended'));

ALTER TABLE appointments
DROP COLUMN IF EXISTS payment_due_at,
DROP COLUMN IF EXISTS payment_status;
