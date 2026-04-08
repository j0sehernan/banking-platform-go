-- transaction_explanations: cache of the explanations generated
-- automatically when consuming TransactionCompleted/Rejected.
-- Avoids hitting Claude every time the front asks for the explanation.

CREATE TABLE transaction_explanations (
    tx_id         UUID PRIMARY KEY,
    explanation   TEXT NOT NULL,
    model         VARCHAR(64) NOT NULL,
    generated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- transactions_view: materialized view of transactions, fed by Kafka
-- events. This is what the chat uses to build the transaction context
-- without touching transactions-ms via HTTP.
--
-- Pattern: Materialized View / Read Model. Each service keeps the data
-- it needs in its own storage, fed by events.

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
