CREATE TABLE prescription_items (
    id BIGSERIAL PRIMARY KEY,
    medical_record_id BIGINT NOT NULL,
    position SMALLINT NOT NULL,

    medicine_name VARCHAR(200) NOT NULL,
    strength VARCHAR(50) NOT NULL DEFAULT '',
    form VARCHAR(30) NOT NULL DEFAULT '',
    dosage VARCHAR(50) NOT NULL,
    frequency VARCHAR(50) NOT NULL DEFAULT '',
    duration_days SMALLINT NOT NULL DEFAULT 0 CHECK (duration_days >= 0),
    timing VARCHAR(30) NOT NULL DEFAULT 'any' CHECK (timing IN ('before_food', 'after_food', 'with_food', 'any')),
    instructions TEXT NOT NULL DEFAULT '',

    FOREIGN KEY (medical_record_id) REFERENCES medical_records(id) ON DELETE CASCADE
);

CREATE INDEX idx_prescription_items_medical_record_id_position
    ON prescription_items (medical_record_id, position);
