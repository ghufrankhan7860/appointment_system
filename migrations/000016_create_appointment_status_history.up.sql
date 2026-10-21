CREATE TABLE appointment_status_history (
    id BIGSERIAL PRIMARY KEY,
    appointment_id BIGINT NOT NULL,
    changed_by_user_id BIGINT NOT NULL,

    from_status VARCHAR(30) NOT NULL,
    to_status VARCHAR(30) NOT NULL,
    reason TEXT NOT NULL DEFAULT '',

    created_at TIMESTAMPTZ NOT NULL,

    FOREIGN KEY (appointment_id) REFERENCES appointments(id),
    FOREIGN KEY (changed_by_user_id) REFERENCES users(id)
);

CREATE INDEX idx_appointment_status_history_appointment_id_created_at
    ON appointment_status_history (appointment_id, created_at);
