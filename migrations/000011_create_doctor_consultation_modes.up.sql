CREATE TABLE doctor_consultation_modes (
    doctor_id BIGINT NOT NULL,

    mode VARCHAR(30) NOT NULL CHECK (mode IN ('in_person', 'video', 'phone')),

    PRIMARY KEY (doctor_id, mode),

    FOREIGN KEY (doctor_id) REFERENCES doctors(id) ON DELETE CASCADE
);

CREATE INDEX idx_doctor_consultation_modes_mode
    ON doctor_consultation_modes (mode);