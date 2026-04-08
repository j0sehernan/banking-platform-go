-- transaction_explanations: cache de las explicaciones generadas
-- automáticamente al consumir TransactionCompleted/Rejected.
-- Esto evita llamar a Claude cada vez que el front pide la explicación.

CREATE TABLE transaction_explanations (
    tx_id         UUID PRIMARY KEY,
    explanation   TEXT NOT NULL,
    model         VARCHAR(64) NOT NULL,
    generated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- transactions_view: vista materializada de transacciones, alimentada
-- por los eventos de Kafka. Es lo que el chat usa para construir el
-- contexto de la transacción sin tocar a transactions-ms vía HTTP.
--
-- Patrón: Materialized View / Read Model. Cada servicio mantiene los
-- datos que necesita en su propio storage, alimentado por eventos.

CREATE TABLE transactions_view (
    id                UUID PRIMARY KEY,
    type              VARCHAR(20) NOT NULL,
    status            VARCHAR(20) NOT NULL,
    from_account_id   UUID,
    to_account_id     UUID,
    amount            NUMERIC(20, 4) NOT NULL,
    currency          CHAR(3) NOT NULL,
    rejection_code    VARCHAR(64),
    rejection_msg     TEXT,
    created_at        TIMESTAMPTZ NOT NULL,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_tx_view_status ON transactions_view(status);
