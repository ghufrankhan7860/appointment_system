CREATE TABLE patient_invite_attempts (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,

    created_at TIMESTAMPTZ NOT NULL,

    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX idx_patient_invite_attempts_user_id_created_at
    ON patient_invite_attempts (user_id, created_at);
