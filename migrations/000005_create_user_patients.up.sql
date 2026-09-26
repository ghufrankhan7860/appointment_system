CREATE TABLE user_patient (
    user_id BIGINT NOT NULL,
    patient_id BIGINT NOT NULL,

    patient_relationship VARCHAR(30) NOT NULL CHECK (patient_relationship IN ('self', 'father', 'mother', 'spouse', 'child', 'sibling', 'guardian', 'caregiver', 'other')),
    access_level VARCHAR(30) NOT NULL CHECK (access_level IN ('full', 'appointments_only', 'read_only')),
    status VARCHAR(30) NOT NULL CHECK (status IN ('pending', 'active', 'revoked')),

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    PRIMARY KEY (user_id, patient_id),

    FOREIGN KEY (user_id) REFERENCES users(id),
    FOREIGN KEY (patient_id) REFERENCES patients(id)
);