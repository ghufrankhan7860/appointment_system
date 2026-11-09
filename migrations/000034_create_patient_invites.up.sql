CREATE TABLE patient_invites (
    id BIGSERIAL PRIMARY KEY,
    patient_id BIGINT NOT NULL,
    created_by_user_id BIGINT NOT NULL,

    code_hash VARCHAR(64) NOT NULL,
    patient_relationship VARCHAR(30) NOT NULL CHECK (patient_relationship IN ('self', 'father', 'mother', 'spouse', 'child', 'sibling', 'guardian', 'caregiver', 'other')),
    access_level VARCHAR(30) NOT NULL CHECK (access_level IN ('full', 'appointments_only', 'read_only')),
    status VARCHAR(30) NOT NULL CHECK (status IN ('pending', 'redeemed', 'revoked', 'expired')),

    expires_at TIMESTAMPTZ NOT NULL,
    redeemed_by_user_id BIGINT NULL,
    redeemed_at TIMESTAMPTZ NULL,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT patient_invites_self_full_check CHECK (patient_relationship <> 'self' OR access_level = 'full'),

    FOREIGN KEY (patient_id) REFERENCES patients(id),
    FOREIGN KEY (created_by_user_id) REFERENCES users(id),
    FOREIGN KEY (redeemed_by_user_id) REFERENCES users(id)
);

CREATE UNIQUE INDEX idx_patient_invites_code_hash_unique
    ON patient_invites (code_hash);

CREATE INDEX idx_patient_invites_patient_id_status
    ON patient_invites (patient_id, status);
