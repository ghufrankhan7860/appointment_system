CREATE TABLE refunds (
    id BIGSERIAL PRIMARY KEY,
    payment_id BIGINT NOT NULL,
    initiated_by_user_id BIGINT NULL,

    amount INTEGER NOT NULL CHECK (amount > 0),
    reason VARCHAR(30) NOT NULL CHECK (reason IN ('patient_cancelled', 'doctor_cancelled', 'late_payment', 'admin')),
    note TEXT NOT NULL DEFAULT '',

    provider_refund_id VARCHAR(100) NULL,
    status VARCHAR(30) NOT NULL CHECK (status IN ('requested', 'succeeded', 'failed')),
    processed_at TIMESTAMPTZ NULL,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    FOREIGN KEY (payment_id) REFERENCES payments(id),
    FOREIGN KEY (initiated_by_user_id) REFERENCES users(id)
);

CREATE UNIQUE INDEX idx_refunds_payment_id_unique
    ON refunds (payment_id)
    WHERE status <> 'failed';

CREATE INDEX idx_refunds_status_created_at
    ON refunds (status, created_at);
