-- Inbox pattern (idempotencia de consumers): guarda IDs de eventos
-- ya procesados. Antes de manejar un evento, hacemos INSERT con
-- ON CONFLICT DO NOTHING. Si no devuelve fila → ya lo procesamos, skip.
--
-- Sin esta tabla, un re-delivery de Kafka (que ocurre normalmente)
-- causaría doble débito en una cuenta. Catastrófico en banca.

CREATE TABLE processed_events (
    event_id      UUID PRIMARY KEY,
    processed_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
