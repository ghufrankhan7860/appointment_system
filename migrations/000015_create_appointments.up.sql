CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE appointments (
    id BIGSERIAL PRIMARY KEY,
    patient_id BIGINT NOT NULL,
    doctor_id BIGINT NOT NULL,
    booked_by_user_id BIGINT NOT NULL,

    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    mode VARCHAR(30) NOT NULL CHECK (mode IN ('in_person', 'video', 'phone')),
    status VARCHAR(30) NOT NULL CHECK (status IN ('scheduled', 'completed', 'cancelled', 'no_show')),

    consultation_fee INTEGER NOT NULL CHECK (consultation_fee >= 0),
    slot_duration_min SMALLINT NOT NULL CHECK (slot_duration_min > 0),
    patient_notes TEXT NOT NULL DEFAULT '',

    cancelled_at TIMESTAMPTZ NULL,
    cancelled_by_user_id BIGINT NULL,
    cancel_reason VARCHAR(30) NULL CHECK (cancel_reason IN ('patient_request', 'doctor_request', 'admin', 'rescheduled', 'doctor_suspended')),

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    CHECK (ends_at > starts_at),

    CONSTRAINT appointments_doctor_no_overlap EXCLUDE USING gist (
        doctor_id WITH =,
        tstzrange(starts_at, ends_at, '[)') WITH &&
    ) WHERE (status = 'scheduled'),

    FOREIGN KEY (patient_id) REFERENCES patients(id),
    FOREIGN KEY (doctor_id) REFERENCES doctors(id),
    FOREIGN KEY (booked_by_user_id) REFERENCES users(id),
    FOREIGN KEY (cancelled_by_user_id) REFERENCES users(id)
);

CREATE INDEX idx_appointments_doctor_id_starts_at
    ON appointments (doctor_id, starts_at);

CREATE INDEX idx_appointments_patient_id_starts_at
    ON appointments (patient_id, starts_at);
