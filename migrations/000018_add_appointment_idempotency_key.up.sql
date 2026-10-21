ALTER TABLE appointments
ADD COLUMN idempotency_key VARCHAR(100) NULL;

CREATE UNIQUE INDEX idx_appointments_booked_by_user_id_idempotency_key_unique
    ON appointments (booked_by_user_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
