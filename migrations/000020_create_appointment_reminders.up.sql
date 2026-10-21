CREATE TABLE appointment_reminders (
    appointment_id BIGINT NOT NULL,

    reminder_window VARCHAR(30) NOT NULL CHECK (reminder_window IN ('day_before', 'two_hours')),

    created_at TIMESTAMPTZ NOT NULL,

    PRIMARY KEY (appointment_id, reminder_window),

    FOREIGN KEY (appointment_id) REFERENCES appointments(id)
);
