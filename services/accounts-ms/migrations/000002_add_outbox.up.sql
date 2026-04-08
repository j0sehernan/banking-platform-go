-- Outbox pattern: garantiza atomicidad DB+evento sin 2PC.
-- Las inserciones a esta tabla ocurren en la misma transacción
-- que el cambio de estado del dominio. Un worker en background
-- las lee y las publica a Kafka.

CREATE TABLE outbox (
    id            UUID PRIMARY KEY,
    topic         VARCHAR(255) NOT NULL,
    msg_key       VARCHAR(255) NOT NULL,
    payload       JSONB NOT NULL,
    headers       JSONB,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at  TIMESTAMPTZ
);

-- Índice parcial: solo eventos pendientes (más eficiente que un índice completo).
CREATE INDEX idx_outbox_unpublished
    ON outbox(created_at)
    WHERE published_at IS NULL;
