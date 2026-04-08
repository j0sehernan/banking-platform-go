-- Tabla principal de transacciones.
--
-- Constraints clave:
--  * idempotency_key UNIQUE → garantiza no duplicados (cliente puede reintentar)
--  * CHECK (amount > 0)     → no permite montos cero o negativos
--  * CHECK (status IN ...)  → enforce de la máquina de estados a nivel DB
--  * CHECK (type IN ...)    → enforce de tipos válidos

CREATE TABLE transactions (
    id               UUID PRIMARY KEY,
    type             VARCHAR(20) NOT NULL CHECK (type IN ('DEPOSIT', 'WITHDRAW', 'TRANSFER')),
    from_account_id  UUID,
    to_account_id    UUID,
    amount           NUMERIC(20, 4) NOT NULL CHECK (amount > 0),
    currency         CHAR(3) NOT NULL CHECK (currency IN ('USD', 'EUR', 'ARS')),
    status           VARCHAR(20) NOT NULL CHECK (status IN ('PENDING', 'COMPLETED', 'REJECTED')),
    idempotency_key  VARCHAR(128) NOT NULL UNIQUE,
    rejection_code   VARCHAR(64),
    rejection_msg    TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_transactions_from ON transactions(from_account_id) WHERE from_account_id IS NOT NULL;
CREATE INDEX idx_transactions_to   ON transactions(to_account_id)   WHERE to_account_id IS NOT NULL;
CREATE INDEX idx_transactions_status ON transactions(status);
