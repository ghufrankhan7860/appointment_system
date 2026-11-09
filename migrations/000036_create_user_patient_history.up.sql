CREATE TABLE user_patient_history (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    patient_id BIGINT NOT NULL,
    changed_by_user_id BIGINT NOT NULL,

    from_status VARCHAR(30) NOT NULL,
    to_status VARCHAR(30) NOT NULL,
    from_access_level VARCHAR(30) NOT NULL,
    to_access_level VARCHAR(30) NOT NULL,
    reason VARCHAR(30) NOT NULL CHECK (reason IN ('created', 'invite_redeemed', 'access_changed', 'revoked_by_holder', 'left')),

    created_at TIMESTAMPTZ NOT NULL,

    FOREIGN KEY (user_id, patient_id) REFERENCES user_patient(user_id, patient_id),
    FOREIGN KEY (changed_by_user_id) REFERENCES users(id)
);

CREATE INDEX idx_user_patient_history_patient_id_created_at
    ON user_patient_history (patient_id, created_at DESC);
