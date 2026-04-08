-- Outbox pattern: guarantees DB+event atomicity without 2PC.
-- Inserts to this table happen in the same transaction as the domain
-- state change. A background worker reads them and publishes to Kafka.

CREATE TABLE outbox (
    id            UUID PRIMARY KEY,
    topic         VARCHAR(255) NOT NULL,
    msg_key       VARCHAR(255) NOT NULL,
    payload       JSONB NOT NULL,
    headers       JSONB,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at  TIMESTAMPTZ
);

-- Partial index: only pending events (more efficient than a full index).
CREATE INDEX idx_outbox_unpublished
    ON outbox(created_at)
    WHERE published_at IS NULL;
