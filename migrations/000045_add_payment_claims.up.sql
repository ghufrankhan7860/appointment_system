ALTER TABLE payments
DROP CONSTRAINT IF EXISTS payments_provider_check;

ALTER TABLE payments
ADD CONSTRAINT payments_provider_check CHECK (provider IN ('none', 'razorpay', 'upi_direct', 'clinic'));

ALTER TABLE payments
DROP CONSTRAINT IF EXISTS payments_status_check;

ALTER TABLE payments
ADD CONSTRAINT payments_status_check CHECK (status IN ('created', 'captured', 'failed', 'expired', 'refunded', 'claimed', 'rejected'));

ALTER TABLE payments
ADD COLUMN payer_reference VARCHAR(35) NULL,
ADD COLUMN proof_storage_key VARCHAR(255) NULL,
ADD COLUMN claimed_at TIMESTAMPTZ NULL,
ADD COLUMN reviewed_by_user_id BIGINT NULL REFERENCES users(id),
ADD COLUMN reviewed_at TIMESTAMPTZ NULL,
ADD COLUMN review_note TEXT NOT NULL DEFAULT '';

DROP INDEX IF EXISTS idx_payments_appointment_id_live_unique;

CREATE UNIQUE INDEX idx_payments_appointment_id_live_unique
    ON payments (appointment_id)
    WHERE status IN ('created', 'captured', 'claimed');

CREATE INDEX idx_payments_status_claimed_at
    ON payments (claimed_at)
    WHERE status = 'claimed';
