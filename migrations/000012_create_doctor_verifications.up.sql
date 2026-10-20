CREATE TABLE doctor_verifications (
    id BIGSERIAL PRIMARY KEY,
    doctor_id BIGINT NOT NULL,
    reviewed_by_user_id BIGINT NOT NULL,

    from_status VARCHAR(30) NOT NULL,
    to_status VARCHAR(30) NOT NULL,
    reason TEXT NOT NULL DEFAULT '',

    created_at TIMESTAMPTZ NOT NULL,

    FOREIGN KEY (doctor_id) REFERENCES doctors(id) ON DELETE CASCADE,
    FOREIGN KEY (reviewed_by_user_id) REFERENCES users(id)
);

CREATE INDEX idx_doctor_verifications_doctor_id_created_at
    ON doctor_verifications (doctor_id, created_at DESC);