ALTER TABLE appointments
ADD COLUMN payment_status VARCHAR(30) NOT NULL DEFAULT 'waived' CHECK (payment_status IN ('unpaid', 'paid', 'refunded', 'waived')),
ADD COLUMN payment_due_at TIMESTAMPTZ NULL;

ALTER TABLE appointments
ALTER COLUMN payment_status DROP DEFAULT;

ALTER TABLE appointments
DROP CONSTRAINT IF EXISTS appointments_cancel_reason_check;

ALTER TABLE appointments
ADD CONSTRAINT appointments_cancel_reason_check CHECK (cancel_reason IN ('patient_request', 'doctor_request', 'admin', 'rescheduled', 'doctor_suspended', 'payment_expired'));

CREATE INDEX idx_appointments_payment_due_at
    ON appointments (payment_due_at)
    WHERE payment_status = 'unpaid';

ALTER TABLE appointment_status_history
ALTER COLUMN changed_by_user_id DROP NOT NULL;
