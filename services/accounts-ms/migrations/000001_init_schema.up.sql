-- Initial schema: clients and accounts.
-- The CHECK (balance >= 0) is the last line of defense: even with a bug
-- in the code, Postgres rejects the UPDATE if the balance would go
-- negative. Cannot be bypassed from the app.

CREATE TABLE clients (
    id          UUID PRIMARY KEY,
    name        VARCHAR(255) NOT NULL,
    email       VARCHAR(255) UNIQUE NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE accounts (
    id          UUID PRIMARY KEY,
    client_id   UUID NOT NULL REFERENCES clients(id) ON DELETE RESTRICT,
    currency    CHAR(3) NOT NULL CHECK (currency IN ('USD', 'EUR', 'ARS')),
    balance     NUMERIC(20, 4) NOT NULL DEFAULT 0 CHECK (balance >= 0),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_accounts_client_id ON accounts(client_id);
