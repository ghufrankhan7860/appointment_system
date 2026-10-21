CREATE TABLE appointment_reschedule_proposals (
    id BIGSERIAL PRIMARY KEY,
    appointment_id BIGINT NOT NULL,
    placeholder_appointment_id BIGINT NOT NULL,
    proposed_by_user_id BIGINT NOT NULL,

    reason VARCHAR(30) NOT NULL CHECK (reason IN ('emergency', 'personal', 'clinic_closed', 'other')),
    note TEXT NOT NULL DEFAULT '',
    status VARCHAR(30) NOT NULL CHECK (status IN ('pending', 'accepted', 'declined', 'expired')),
    expires_at TIMESTAMPTZ NOT NULL,

    responded_at TIMESTAMPTZ NULL,
    responded_by_user_id BIGINT NULL,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    FOREIGN KEY (appointment_id) REFERENCES appointments(id),
    FOREIGN KEY (placeholder_appointment_id) REFERENCES appointments(id),
    FOREIGN KEY (proposed_by_user_id) REFERENCES users(id),
    FOREIGN KEY (responded_by_user_id) REFERENCES users(id)
);

CREATE UNIQUE INDEX idx_reschedule_proposals_appointment_id_pending_unique
    ON appointment_reschedule_proposals (appointment_id)
    WHERE status = 'pending';

CREATE INDEX idx_appointment_reschedule_proposals_expires_at
    ON appointment_reschedule_proposals (expires_at)
    WHERE status = 'pending';
