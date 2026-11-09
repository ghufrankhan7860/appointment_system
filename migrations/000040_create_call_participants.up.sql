CREATE TABLE call_participants (
    id BIGSERIAL PRIMARY KEY,
    call_room_id BIGINT NOT NULL,
    user_id BIGINT NULL,
    guest_token_id BIGINT NULL,

    peer_id VARCHAR(32) NOT NULL,
    display_name VARCHAR(100) NOT NULL,
    role VARCHAR(30) NOT NULL CHECK (role IN ('host', 'doctor', 'patient_side', 'guest', 'member')),
    admission_status VARCHAR(30) NOT NULL CHECK (admission_status IN ('admitted', 'waiting', 'denied', 'removed')),
    media_mode VARCHAR(30) NOT NULL CHECK (media_mode IN ('audio', 'video')),
    muted_by_host BOOLEAN NOT NULL,
    connection_state VARCHAR(30) NOT NULL DEFAULT '' CHECK (connection_state IN ('', 'connected', 'relay', 'failed')),

    joined_at TIMESTAMPTZ NULL,
    last_seen_at TIMESTAMPTZ NULL,
    left_at TIMESTAMPTZ NULL,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    FOREIGN KEY (call_room_id) REFERENCES call_rooms(id),
    FOREIGN KEY (user_id) REFERENCES users(id),
    FOREIGN KEY (guest_token_id) REFERENCES call_guest_tokens(id)
);

CREATE UNIQUE INDEX idx_call_participants_call_room_id_peer_id_unique
    ON call_participants (call_room_id, peer_id);

CREATE UNIQUE INDEX idx_call_participants_call_room_id_user_id_unique
    ON call_participants (call_room_id, user_id)
    WHERE user_id IS NOT NULL;
