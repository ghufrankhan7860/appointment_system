ALTER TABLE appointments
DROP CONSTRAINT IF EXISTS appointments_status_check;

ALTER TABLE appointments
ADD CONSTRAINT appointments_status_check CHECK (status IN ('scheduled', 'completed', 'cancelled', 'no_show', 'reschedule_proposed'));

ALTER TABLE appointments
DROP CONSTRAINT IF EXISTS appointments_cancel_reason_check;

ALTER TABLE appointments
ADD CONSTRAINT appointments_cancel_reason_check CHECK (cancel_reason IN ('patient_request', 'doctor_request', 'admin', 'rescheduled', 'doctor_suspended', 'payment_expired', 'proposal_declined', 'proposal_expired'));

ALTER TABLE appointments
DROP CONSTRAINT IF EXISTS appointments_doctor_no_overlap;

ALTER TABLE appointments
ADD CONSTRAINT appointments_doctor_no_overlap EXCLUDE USING gist (
    doctor_id WITH =,
    tstzrange(starts_at, ends_at, '[)') WITH &&
) WHERE (status IN ('scheduled', 'reschedule_proposed'));
