-- Schema inicial: clientes y cuentas.
-- El CHECK (balance >= 0) es la última línea de defensa: aunque haya
-- un bug en el código, Postgres rechaza el UPDATE si el saldo quedaría
-- negativo. Imposible bypassearlo desde la app.

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
