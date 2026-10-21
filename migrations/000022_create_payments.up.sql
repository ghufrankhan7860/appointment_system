CREATE TABLE payments (
    id BIGSERIAL PRIMARY KEY,
    appointment_id BIGINT NOT NULL,
    payer_user_id BIGINT NOT NULL,

    amount INTEGER NOT NULL CHECK (amount > 0),
    currency CHAR(3) NOT NULL,

    provider VARCHAR(30) NOT NULL CHECK (provider IN ('none', 'razorpay')),
    provider_order_id VARCHAR(100) NOT NULL,
    provider_payment_id VARCHAR(100) NULL,

    status VARCHAR(30) NOT NULL CHECK (status IN ('created', 'captured', 'failed', 'expired', 'refunded')),
    captured_at TIMESTAMPTZ NULL,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    UNIQUE (provider, provider_order_id),

    FOREIGN KEY (appointment_id) REFERENCES appointments(id),
    FOREIGN KEY (payer_user_id) REFERENCES users(id)
);

CREATE INDEX idx_payments_appointment_id
    ON payments (appointment_id);

CREATE UNIQUE INDEX idx_payments_appointment_id_live_unique
    ON payments (appointment_id)
    WHERE status IN ('created', 'captured');

CREATE UNIQUE INDEX idx_payments_provider_provider_payment_id_unique
    ON payments (provider, provider_payment_id)
    WHERE provider_payment_id IS NOT NULL;
