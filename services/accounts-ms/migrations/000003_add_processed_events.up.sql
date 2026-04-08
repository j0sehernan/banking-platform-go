-- Inbox pattern (consumer idempotency): keeps the IDs of already
-- processed events. Before handling an event we INSERT with
-- ON CONFLICT DO NOTHING. If no row is returned → already processed, skip.
--
-- Without this table, a Kafka redelivery (which happens normally)
-- would cause double debit on an account. Catastrophic in banking.

CREATE TABLE processed_events (
    event_id      UUID PRIMARY KEY,
    processed_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
