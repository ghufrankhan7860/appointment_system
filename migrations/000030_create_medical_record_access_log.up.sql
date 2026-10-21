CREATE TABLE medical_record_access_log (
    id BIGSERIAL PRIMARY KEY,
    patient_id BIGINT NOT NULL,
    medical_record_id BIGINT NULL,
    attachment_id BIGINT NULL,
    accessed_by_user_id BIGINT NOT NULL,

    action VARCHAR(30) NOT NULL CHECK (action IN ('create', 'update', 'finalize', 'view', 'list', 'upload', 'download')),
    correlation_id VARCHAR(100) NOT NULL DEFAULT '',

    created_at TIMESTAMPTZ NOT NULL,

    FOREIGN KEY (patient_id) REFERENCES patients(id),
    FOREIGN KEY (medical_record_id) REFERENCES medical_records(id),
    FOREIGN KEY (attachment_id) REFERENCES medical_attachments(id),
    FOREIGN KEY (accessed_by_user_id) REFERENCES users(id)
);

CREATE INDEX idx_medical_record_access_log_medical_record_id_created_at
    ON medical_record_access_log (medical_record_id, created_at);

CREATE INDEX idx_medical_record_access_log_patient_id_created_at
    ON medical_record_access_log (patient_id, created_at);
