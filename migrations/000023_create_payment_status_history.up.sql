CREATE TABLE payment_status_history (
    id BIGSERIAL PRIMARY KEY,
    payment_id BIGINT NOT NULL,
    changed_by_user_id BIGINT NULL,

    from_status VARCHAR(30) NOT NULL,
    to_status VARCHAR(30) NOT NULL,
    reason TEXT NOT NULL DEFAULT '',

    created_at TIMESTAMPTZ NOT NULL,

    FOREIGN KEY (payment_id) REFERENCES payments(id),
    FOREIGN KEY (changed_by_user_id) REFERENCES users(id)
);

CREATE INDEX idx_payment_status_history_payment_id_created_at
    ON payment_status_history (payment_id, created_at);
