CREATE TABLE medical_records (
    id BIGSERIAL PRIMARY KEY,
    appointment_id BIGINT NOT NULL UNIQUE,
    patient_id BIGINT NOT NULL,
    doctor_id BIGINT NOT NULL,
    created_by_user_id BIGINT NOT NULL,

    chief_complaint TEXT NOT NULL DEFAULT '',
    symptoms TEXT NOT NULL DEFAULT '',
    diagnosis TEXT NOT NULL DEFAULT '',
    examination_notes TEXT NOT NULL DEFAULT '',
    advice TEXT NOT NULL DEFAULT '',
    prescription_notes TEXT NOT NULL DEFAULT '',
    follow_up_on DATE NULL,
    vitals JSONB NOT NULL DEFAULT '{}',

    status VARCHAR(30) NOT NULL CHECK (status IN ('draft', 'finalized')),
    finalized_at TIMESTAMPTZ NULL,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    FOREIGN KEY (appointment_id) REFERENCES appointments(id),
    FOREIGN KEY (patient_id) REFERENCES patients(id),
    FOREIGN KEY (doctor_id) REFERENCES doctors(id),
    FOREIGN KEY (created_by_user_id) REFERENCES users(id)
);

CREATE INDEX idx_medical_records_patient_id_created_at
    ON medical_records (patient_id, created_at);
