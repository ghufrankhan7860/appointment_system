CREATE TABLE outbox (
    id BIGSERIAL PRIMARY KEY,

    aggregate_type VARCHAR(30) NOT NULL,
    aggregate_id BIGINT NOT NULL,
    event_type VARCHAR(50) NOT NULL,
    payload JSONB NOT NULL,

    created_at TIMESTAMPTZ NOT NULL,
    published_at TIMESTAMPTZ NULL
);

CREATE INDEX idx_outbox_published_at_id
    ON outbox (published_at, id);
