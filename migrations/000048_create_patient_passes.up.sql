-- The rotating code a patient shows at the clinic desk. One live pass per
-- patient; only hashes of the QR token and the typed code are kept.
CREATE TABLE patient_passes (
    id BIGSERIAL PRIMARY KEY,
    patient_id BIGINT NOT NULL,
    created_by_user_id BIGINT NOT NULL,

    token_hash VARCHAR(64) NOT NULL,
    code_hash VARCHAR(64) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,

    created_at TIMESTAMPTZ NOT NULL,

    FOREIGN KEY (patient_id) REFERENCES patients(id),
    FOREIGN KEY (created_by_user_id) REFERENCES users(id)
);

CREATE UNIQUE INDEX idx_patient_passes_patient_id_unique
    ON patient_passes (patient_id);

CREATE UNIQUE INDEX idx_patient_passes_token_hash_unique
    ON patient_passes (token_hash);

CREATE UNIQUE INDEX idx_patient_passes_code_hash_unique
    ON patient_passes (code_hash);

CREATE TABLE patient_pass_attempts (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,

    created_at TIMESTAMPTZ NOT NULL,

    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX idx_patient_pass_attempts_user_id_created_at
    ON patient_pass_attempts (user_id, created_at);

-- Patients a doctor's desk may book for: shown the patient's pass, or added
-- the patient as a walk-in.
CREATE TABLE clinic_patients (
    doctor_id BIGINT NOT NULL,
    patient_id BIGINT NOT NULL,

    source VARCHAR(20) NOT NULL CHECK (source IN ('pass', 'walk_in')),
    created_by_user_id BIGINT NOT NULL,

    created_at TIMESTAMPTZ NOT NULL,

    PRIMARY KEY (doctor_id, patient_id),
    FOREIGN KEY (doctor_id) REFERENCES doctors(id),
    FOREIGN KEY (patient_id) REFERENCES patients(id),
    FOREIGN KEY (created_by_user_id) REFERENCES users(id)
);

CREATE INDEX idx_clinic_patients_patient_id
    ON clinic_patients (patient_id);
