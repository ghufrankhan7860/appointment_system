CREATE TABLE medical_attachments (
    id BIGSERIAL PRIMARY KEY,
    patient_id BIGINT NOT NULL,
    medical_record_id BIGINT NULL,
    uploaded_by_user_id BIGINT NOT NULL,

    kind VARCHAR(30) NOT NULL CHECK (kind IN ('lab_report', 'imaging', 'prescription_scan', 'referral', 'other')),
    title VARCHAR(200) NOT NULL,
    storage_key VARCHAR(255) NOT NULL UNIQUE,
    content_type VARCHAR(100) NOT NULL,
    size_bytes BIGINT NOT NULL CHECK (size_bytes > 0),
    taken_on DATE NULL,

    created_at TIMESTAMPTZ NOT NULL,

    FOREIGN KEY (patient_id) REFERENCES patients(id),
    FOREIGN KEY (medical_record_id) REFERENCES medical_records(id),
    FOREIGN KEY (uploaded_by_user_id) REFERENCES users(id)
);

CREATE INDEX idx_medical_attachments_patient_id_created_at
    ON medical_attachments (patient_id, created_at);

CREATE INDEX idx_medical_attachments_medical_record_id
    ON medical_attachments (medical_record_id);
