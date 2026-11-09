CREATE TABLE call_join_attempts (
    id BIGSERIAL PRIMARY KEY,
    client_ip VARCHAR(45) NOT NULL,

    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_call_join_attempts_client_ip_created_at
    ON call_join_attempts (client_ip, created_at);
