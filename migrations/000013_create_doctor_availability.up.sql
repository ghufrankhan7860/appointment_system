CREATE TABLE doctor_availability (
    id BIGSERIAL PRIMARY KEY,
    doctor_id BIGINT NOT NULL,

    day_of_week SMALLINT NOT NULL CHECK (day_of_week BETWEEN 0 AND 6),
    start_minute SMALLINT NOT NULL CHECK (start_minute BETWEEN 0 AND 1439),
    end_minute SMALLINT NOT NULL CHECK (end_minute BETWEEN 1 AND 1440),
    mode VARCHAR(30) NOT NULL DEFAULT 'in_person' CHECK (mode IN ('in_person', 'video', 'phone')),

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    CHECK (end_minute > start_minute),

    FOREIGN KEY (doctor_id) REFERENCES doctors(id) ON DELETE CASCADE
);

CREATE INDEX idx_doctor_availability_doctor_id_day_of_week
    ON doctor_availability (doctor_id, day_of_week);