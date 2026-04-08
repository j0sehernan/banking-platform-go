CREATE TABLE outbox (
    id            UUID PRIMARY KEY,
    topic         VARCHAR(255) NOT NULL,
    msg_key       VARCHAR(255) NOT NULL,
    payload       JSONB NOT NULL,
    headers       JSONB,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at  TIMESTAMPTZ
);

CREATE INDEX idx_outbox_unpublished
    ON outbox(created_at)
    WHERE published_at IS NULL;
