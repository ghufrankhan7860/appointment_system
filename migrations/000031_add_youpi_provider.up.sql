ALTER TABLE payments
DROP CONSTRAINT IF EXISTS payments_provider_check;

ALTER TABLE payments
ADD CONSTRAINT payments_provider_check CHECK (provider IN ('none', 'razorpay', 'youpi'));

CREATE INDEX idx_refunds_status_created_at
    ON refunds (status, created_at);
