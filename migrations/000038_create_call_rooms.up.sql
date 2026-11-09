CREATE TABLE call_rooms (
    id BIGSERIAL PRIMARY KEY,
    kind VARCHAR(30) NOT NULL CHECK (kind IN ('appointment', 'code')),
    appointment_id BIGINT NULL,
    host_user_id BIGINT NOT NULL,

    title VARCHAR(100) NOT NULL DEFAULT '',
    join_code_hash VARCHAR(64) NULL,
    max_participants SMALLINT NOT NULL CHECK (max_participants BETWEEN 2 AND 5),
    requires_approval BOOLEAN NOT NULL,
    scheduled_at TIMESTAMPTZ NULL,

    status VARCHAR(30) NOT NULL CHECK (status IN ('open', 'active', 'ended')),
    opened_at TIMESTAMPTZ NOT NULL,
    started_at TIMESTAMPTZ NULL,
    ended_at TIMESTAMPTZ NULL,
    end_reason VARCHAR(30) NOT NULL DEFAULT '' CHECK (end_reason IN ('', 'hangup', 'host_ended', 'timeout', 'appointment_closed', 'no_answer', 'expired')),
    expires_at TIMESTAMPTZ NULL,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT call_rooms_appointment_check CHECK ((kind = 'appointment') = (appointment_id IS NOT NULL)),
    CONSTRAINT call_rooms_join_code_check CHECK ((kind = 'code') = (join_code_hash IS NOT NULL)),

    FOREIGN KEY (appointment_id) REFERENCES appointments(id),
    FOREIGN KEY (host_user_id) REFERENCES users(id)
);

CREATE UNIQUE INDEX idx_call_rooms_appointment_id_live_unique
    ON call_rooms (appointment_id)
    WHERE status IN ('open', 'active');

CREATE UNIQUE INDEX idx_call_rooms_join_code_hash_live_unique
    ON call_rooms (join_code_hash)
    WHERE status IN ('open', 'active');

CREATE INDEX idx_call_rooms_host_user_id_status
    ON call_rooms (host_user_id, status);
