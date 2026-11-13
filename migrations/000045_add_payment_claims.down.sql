DROP INDEX IF EXISTS idx_payments_status_claimed_at;

DROP INDEX IF EXISTS idx_payments_appointment_id_live_unique;

CREATE UNIQUE INDEX idx_payments_appointment_id_live_unique
    ON payments (appointment_id)
    WHERE status IN ('created', 'captured');

ALTER TABLE payments
DROP COLUMN IF EXISTS review_note,
DROP COLUMN IF EXISTS reviewed_at,
DROP COLUMN IF EXISTS reviewed_by_user_id,
DROP COLUMN IF EXISTS claimed_at,
DROP COLUMN IF EXISTS proof_storage_key,
DROP COLUMN IF EXISTS payer_reference;

ALTER TABLE payments
DROP CONSTRAINT IF EXISTS payments_status_check;

ALTER TABLE payments
ADD CONSTRAINT payments_status_check CHECK (status IN ('created', 'captured', 'failed', 'expired', 'refunded'));

ALTER TABLE payments
DROP CONSTRAINT IF EXISTS payments_provider_check;

ALTER TABLE payments
ADD CONSTRAINT payments_provider_check CHECK (provider IN ('none', 'razorpay'));
