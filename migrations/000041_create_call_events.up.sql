CREATE TABLE call_events (
    id BIGSERIAL PRIMARY KEY,
    call_room_id BIGINT NOT NULL,
    call_participant_id BIGINT NULL,

    event_type VARCHAR(30) NOT NULL CHECK (event_type IN ('opened', 'joined', 'left', 'admitted', 'denied', 'removed', 'muted', 'unmuted', 'media_changed', 'connection_state', 'ended')),
    detail VARCHAR(30) NOT NULL DEFAULT '',

    created_at TIMESTAMPTZ NOT NULL,

    FOREIGN KEY (call_room_id) REFERENCES call_rooms(id),
    FOREIGN KEY (call_participant_id) REFERENCES call_participants(id)
);

CREATE INDEX idx_call_events_call_room_id_created_at
    ON call_events (call_room_id, created_at);
