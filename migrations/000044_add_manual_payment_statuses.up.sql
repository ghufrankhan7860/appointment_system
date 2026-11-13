ALTER TABLE appointments
DROP CONSTRAINT IF EXISTS appointments_payment_status_check;

ALTER TABLE appointments
ADD CONSTRAINT appointments_payment_status_check CHECK (payment_status IN ('unpaid', 'paid', 'refunded', 'waived', 'pending_verification', 'due_at_clinic'));

ALTER TABLE appointments
DROP CONSTRAINT IF EXISTS appointments_cancel_reason_check;

ALTER TABLE appointments
ADD CONSTRAINT appointments_cancel_reason_check CHECK (cancel_reason IN ('patient_request', 'doctor_request', 'admin', 'rescheduled', 'doctor_suspended', 'payment_expired', 'proposal_declined', 'proposal_expired', 'payment_unverified', 'payment_rejected'));

DROP INDEX IF EXISTS idx_appointments_payment_due_at;

CREATE INDEX idx_appointments_payment_due_at
    ON appointments (payment_due_at)
    WHERE payment_status IN ('unpaid', 'pending_verification');
