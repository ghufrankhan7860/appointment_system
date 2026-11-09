CREATE TABLE call_guest_tokens (
    id BIGSERIAL PRIMARY KEY,
    appointment_id BIGINT NOT NULL,
    created_by_user_id BIGINT NOT NULL,

    token_hash VARCHAR(64) NOT NULL,
    display_name VARCHAR(100) NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,
    max_uses SMALLINT NOT NULL CHECK (max_uses > 0),
    use_count SMALLINT NOT NULL CHECK (use_count >= 0),
    revoked_at TIMESTAMPTZ NULL,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    CHECK (use_count <= max_uses),

    FOREIGN KEY (appointment_id) REFERENCES appointments(id),
    FOREIGN KEY (created_by_user_id) REFERENCES users(id)
);

CREATE UNIQUE INDEX idx_call_guest_tokens_token_hash_unique
    ON call_guest_tokens (token_hash);

CREATE INDEX idx_call_guest_tokens_appointment_id
    ON call_guest_tokens (appointment_id);
