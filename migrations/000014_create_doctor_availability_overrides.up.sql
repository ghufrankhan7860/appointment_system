CREATE TABLE doctor_availability_overrides (
    id BIGSERIAL PRIMARY KEY,
    doctor_id BIGINT NOT NULL,

    override_date DATE NOT NULL,
    start_minute SMALLINT NULL CHECK (start_minute BETWEEN 0 AND 1439),
    end_minute SMALLINT NULL CHECK (end_minute BETWEEN 1 AND 1440),
    is_available BOOLEAN NOT NULL DEFAULT FALSE,
    mode VARCHAR(30) NOT NULL DEFAULT 'in_person' CHECK (mode IN ('in_person', 'video', 'phone')),
    reason TEXT NOT NULL DEFAULT '',

    created_at TIMESTAMPTZ NOT NULL,

    CHECK ((start_minute IS NULL) = (end_minute IS NULL)),
    CHECK (start_minute IS NULL OR end_minute > start_minute),
    CHECK (NOT is_available OR start_minute IS NOT NULL),

    FOREIGN KEY (doctor_id) REFERENCES doctors(id) ON DELETE CASCADE
);

CREATE INDEX idx_doctor_availability_overrides_doctor_id_override_date
    ON doctor_availability_overrides (doctor_id, override_date);