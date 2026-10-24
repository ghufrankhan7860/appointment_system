DROP INDEX IF EXISTS idx_refunds_status_created_at;

ALTER TABLE payments
DROP CONSTRAINT IF EXISTS payments_provider_check;

ALTER TABLE payments
ADD CONSTRAINT payments_provider_check CHECK (provider IN ('none', 'razorpay'));
