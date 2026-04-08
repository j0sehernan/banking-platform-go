# banking-platform-go

A simplified banking platform built with **Go + Apache Kafka + Claude (LLM)** to solve a financial-services technical challenge.

Three decoupled microservices communicate over an event bus. A fourth component (LLM) consumes events from the bus and generates natural-language explanations. There is also a Next.js frontend that doubles as a demo tool: besides a working UI, it includes a side panel that shows every HTTP request/response in real time, and a chat with the LLM with SSE streaming scoped to a specific transaction.

## Table of contents

- [Overall architecture](#overall-architecture)
- [Tech stack](#tech-stack)
- [How to run everything (a single command)](#how-to-run-everything-a-single-command)
- [Available endpoints](#available-endpoints)
- [Full transfer flow](#full-transfer-flow)
- [Role of the LLM microservice](#role-of-the-llm-microservice)
- [Patterns implemented](#patterns-implemented)
- [Technical decisions and trade-offs](#technical-decisions-and-trade-offs)
- [Repo structure](#repo-structure)
- [Tests](#tests)
- [What I would leave for v2](#what-i-would-leave-for-v2)

## Overall architecture

```
                                  ┌──────────────────────────────┐
                                  │      Apache Kafka 3.9        │
                                  │      (KRaft, no ZK)          │
                                  │                              │
                                  │  Topics:                     │
                                  │   ├ accounts.events          │
                                  │   ├ transactions.commands    │
                                  │   ├ accounts.tx-results      │
                                  │   ├ transactions.events      │
                                  │   └ dlq                      │
                                  └──┬───────────┬───────────┬───┘
                                     │           │           │
              ┌──────────────────────┘           │           └──────────────────────┐
              │                                  │                                  │
              ▼                                  ▼                                  ▼
   ┌────────────────────┐              ┌────────────────────┐             ┌────────────────────┐
   │   accounts-ms      │              │  transactions-ms   │             │      llm-ms        │
   │   :8081            │              │  :8082             │             │   :8083            │
   │                    │              │                    │             │                    │
   │  · clients         │              │  · deposit         │             │  · explanation     │
   │  · accounts        │              │  · withdraw        │             │  · chat (SSE)      │
   │  · no-neg balance  │              │  · transfer        │             │  · materialized    │
   │  · outbox + idemp. │              │  · saga orchestr.  │             │    view of tx      │
   └─────────┬──────────┘              └─────────┬──────────┘             └─────────┬──────────┘
             │                                   │                                  │
             ▼                                   ▼                                  ▼
   ┌────────────────┐                  ┌────────────────┐                ┌────────────────┐
   │ accounts-db    │                  │ transactions-  │                │   llm-db       │
   │ (Postgres 16)  │                  │      db        │                │ (Postgres 16)  │
   │                │                  │ (Postgres 16)  │                │                │
   └────────────────┘                  └────────────────┘                └────────────────┘

                                                                                    ▲
                                                                                    │ HTTPS
                                                                                    │
                                                                          ┌─────────┴─────────┐
                                                                          │   Anthropic API   │
                                                                          │ claude-haiku-4-5  │
                                                                          └───────────────────┘

   ┌──────────────────────────────────────────────────────────────────────────────────┐
   │                          web (Next.js, :3000)                                    │
   │                                                                                  │
   │  · list/create accounts                · technical activity side panel           │
   │  · create transactions                 · chat scoped to a tx with SSE streaming  │
   └──────────────────────────────────────────────────────────────────────────────────┘
```

**Principles:**

- Each microservice has **its own database** (no shared tables).
- Inter-service communication is **always via Kafka**, never direct HTTP.
- Each microservice publishes events to its topics and consumes from the topics it cares about.
- The frontend talks HTTP to each microservice (CORS enabled).

## Tech stack

| Layer | Library | Why |
|---|---|---|
| Backend language | Go 1.23 | What the challenge asks for |
| Event bus | Apache Kafka 3.9 (KRaft mode) | What the challenge asks for, no Zookeeper because it is being deprecated |
| HTTP router | `go-chi/chi/v5` | Minimal, idiomatic Go, compatible with `net/http` standard |
| Postgres client | `jackc/pgx/v5` | No ORM, simple type-safe queries |
| Migrations | `golang-migrate/migrate/v4` | De-facto standard, versioned SQL files |
| Kafka client | `segmentio/kafka-go` | Pure Go without CGO, easy to build in Docker |
| Validation | `go-playground/validator/v10` | Declarative tags on DTOs |
| Decimal | `shopspring/decimal` | Critical in banking to keep precision (NUMERIC in PG) |
| Config | `caarlos0/env/v11` | Loads env vars into a struct via tags |
| LLM | `anthropics/anthropic-sdk-go` | Official Claude SDK |
| Logging | stdlib `log/slog` | Built-in structured logging (Go 1.21+) |
| Frontend | Next.js 15 + React 19 + Tailwind | Modern stack, App Router |
| Frontend state | Zustand | Lightweight global store (~2KB), no boilerplate |
| Bus observability | Kafka UI (provectus) | http://localhost:8090, free and professional |

## How to run everything (a single command)

**Prerequisites:** Docker and Docker Compose v2.

```bash
git clone https://github.com/j0sehernan/banking-platform-go.git
cd banking-platform-go

# Optional: if you have an ANTHROPIC_API_KEY, copy .env.example to .env
# and edit it. Otherwise llm-ms starts with MockExplainer and the
# whole system still works.
cp .env.example .env

# Bring everything up
docker compose up
```

In about ~30 seconds you will have:

| URL | What it is |
|---|---|
| http://localhost:3000 | Next.js frontend |
| http://localhost:8090 | **Kafka UI** (inspect topics, messages, consumer groups) |
| http://localhost:8081 | accounts-ms HTTP API |
| http://localhost:8082 | transactions-ms HTTP API |
| http://localhost:8083 | llm-ms HTTP API |
| `localhost:5433/5434/5435` | Postgres (accounts/transactions/llm), `psql -h localhost -p 5433 -U accounts -d accounts` |

To wipe everything (including volumes):

```bash
docker compose down -v
```

## Available endpoints

### accounts-ms (`:8081`)

| Method | Path | Body |
|---|---|---|
| `POST` | `/clients` | `{ "name": "...", "email": "..." }` |
| `GET` | `/clients/{id}` | — |
| `POST` | `/accounts` | `{ "client_id": "uuid", "currency": "USD" }` |
| `GET` | `/accounts` | — |
| `GET` | `/accounts/{id}` | — |
| `GET` | `/accounts/{id}/balance` | — |

### transactions-ms (`:8082`)

| Method | Path | Body |
|---|---|---|
| `POST` | `/transactions/deposit` | `{ "to_account_id", "amount", "currency", "idempotency_key" }` |
| `POST` | `/transactions/withdraw` | `{ "from_account_id", "amount", "currency", "idempotency_key" }` |
| `POST` | `/transactions/transfer` | `{ "from_account_id", "to_account_id", "amount", "currency", "idempotency_key" }` |
| `GET` | `/transactions` | — |
| `GET` | `/transactions/{id}` | — |
| `GET` | `/accounts/{id}/transactions` | — |

### llm-ms (`:8083`)

| Method | Path | Body |
|---|---|---|
| `GET` | `/transactions/{id}/explanation` | — |
| `POST` | `/chat` | `{ "tx_id", "messages": [{"role","content"}, ...] }` (responds with an SSE stream) |

### Manual example with curl

```bash
# 1. Create a client
curl -X POST localhost:8081/clients \
  -H "Content-Type: application/json" \
  -d '{"name":"Ana Perez","email":"ana@example.com"}'

# Save the returned id, you'll use it below (ANA_ID)

# 2. Create an account
curl -X POST localhost:8081/accounts \
  -H "Content-Type: application/json" \
  -d "{\"client_id\":\"$ANA_ID\",\"currency\":\"USD\"}"

# 3. Deposit
curl -X POST localhost:8082/transactions/deposit \
  -H "Content-Type: application/json" \
  -d "{\"to_account_id\":\"$ACC_ID\",\"amount\":\"1000\",\"currency\":\"USD\",\"idempotency_key\":\"dep-001\"}"

# 4. Check balance (should be 1000 after a few seconds)
curl localhost:8081/accounts/$ACC_ID/balance

# 5. See the explanation generated by Claude
curl localhost:8083/transactions/$TX_ID/explanation
```

## Full transfer flow

Step by step of what happens when a user makes a transfer of Ana → Luis for 300 USD.

```
1. POST /transactions/transfer
   ─────────────────────────────
   The front calls transactions-ms with the transfer body.

2. transactions-ms (in a single PG tx):
   ────────────────────────────────────
   BEGIN
     INSERT transactions (id, status='PENDING', ...)
     INSERT outbox (topic='transactions.commands', payload=TransactionRequested)
   COMMIT
   → 202 Accepted to the client with { id, status: PENDING }

3. outbox-worker for transactions-ms (background):
   ───────────────────────────────────────────────
   Reads pending outbox rows every 500ms with FOR UPDATE SKIP LOCKED.
   Publishes TransactionRequested to the transactions.commands topic.

4. accounts-ms consumes from transactions.commands:
   ───────────────────────────────────────────────
   Verifies idempotency (INSERT processed_events ON CONFLICT DO NOTHING).
   BEGIN
     UPDATE accounts SET balance = balance - 300
       WHERE id = AnaAcc AND balance >= 300   ← atomic, no negative balance possible
     UPDATE accounts SET balance = balance + 300 WHERE id = LuisAcc
     INSERT outbox (topic='accounts.tx-results', payload=AccountsTransferApplied)
     INSERT outbox (topic='accounts.events', payload=BalanceUpdated × 2)
   COMMIT

   If the balance was insufficient, the first UPDATE doesn't affect rows.
   accounts-ms does ROLLBACK and publishes AccountsTransferFailed with reason.

5. outbox-worker for accounts-ms publishes to the bus.

6. transactions-ms consumes from accounts.tx-results:
   ──────────────────────────────────────────────────
   Verifies idempotency.
   BEGIN
     UPDATE transactions SET status = 'COMPLETED' (or REJECTED) WHERE id = txID
     INSERT outbox (topic='transactions.events', payload=TransactionCompleted/Rejected)
   COMMIT

7. outbox-worker for transactions-ms publishes to the bus.

8. llm-ms consumes from transactions.events:
   ─────────────────────────────────────────
   Verifies idempotency.
   UPSERT into transactions_view (keeps its local copy of transactions).
   In background (goroutine): calls Claude → stores explanation.

9. Front (in parallel, polling):
   ─────────────────────────────
   GET /transactions/{id} → status: COMPLETED
   GET /transactions/{id}/explanation → generated text
```

This is an **orchestrated saga** with `transactions-ms` as the implicit orchestrator: it starts (step 1), receives the result from accounts (step 6), and emits the final event (step 7). The "saga loop" closes when transactions-ms receives the response from accounts and can mark the final state.

**Compensation**: since debit and credit happen in a single Postgres transaction in accounts-ms, if the credit on the destination failed, the debit on the source automatically rolls back via `ROLLBACK`. The saga is simple because local atomicity removes the need for complex compensating steps.

## Role of the LLM microservice

`llm-ms` does not contain banking logic. Its job is to **interpret information** and generate natural-language text. It fulfills the challenge requirement of "explaining a banking transaction".

It has three responsibilities:

1. **Maintain a materialized view of transactions (read model)**: consumes `transactions.events` from Kafka and keeps a local copy in `transactions_view`. This lets it answer questions about any transaction without touching transactions-ms via HTTP.

2. **Generate automatic explanations**: every time a `TransactionCompleted` or `TransactionRejected` event arrives, llm-ms calls Claude with the context and stores the text in `transaction_explanations`. When the front asks for the explanation, it is returned cached (no Claude call every time).

3. **Chat scoped to one transaction**: the `POST /chat` endpoint receives the `tx_id` and the chat history from the front, reads the transaction context from its local view, builds a system prompt with context grounding and strict scope rules, and returns an SSE stream with the LLM response. The user can only ask about **that** specific transaction.

**Adapter pattern for the LLM**: the `Explainer` interface has two implementations:

- `ClaudeExplainer`: uses the official Anthropic SDK, model `claude-haiku-4-5` (fast, cheap).
- `MockExplainer`: deterministic template responses. Used automatically when `ANTHROPIC_API_KEY` is empty. Lets the system run end-to-end locally without credentials.

**Why the chat does NOT go through Kafka**: the chat is synchronous communication user↔llm-ms↔Claude, there are no business events that other services need to process. Mixing Kafka here would be over-engineering (see "Sync vs Async" below).

## Patterns implemented

### Outbox pattern
Guarantees atomicity between DB state changes and publishing events to Kafka, without two-phase commit. The state change and the outbox row are inserted in the same Postgres transaction. A background worker reads with `FOR UPDATE SKIP LOCKED` and publishes to Kafka. If Kafka is down, events accumulate in outbox and get published when it comes back.

Without this, a crash right after a commit but before publishing would lose the event → inconsistent state across services. **Unacceptable in banking.**

### Inbox pattern (consumer idempotency)
Kafka guarantees at-least-once delivery, which means the same event can arrive twice or more (rebalance, restart, commit that fails). If we don't handle idempotency, a redelivery would cause double debit on an account.

Each consumer does `INSERT INTO processed_events (event_id) ON CONFLICT DO NOTHING RETURNING event_id`. If no row is returned, the event was already processed and gets skipped.

### Orchestrated saga
The transfer is an operation that crosses two microservices (transactions-ms and accounts-ms), so we cannot do it in a single ACID transaction. We model it as a saga: sequential steps coordinated by events, with `transactions-ms` as the implicit orchestrator.

### Materialized view / read model
`llm-ms` does not own transaction data — but it needs it to power the chat. It keeps a local copy (`transactions_view`) fed by Kafka events. This removes any HTTP coupling between llm-ms and transactions-ms, and gives it resilience: even if transactions-ms is down, the chat still works.

### Retries with exponential backoff + DLQ
The consumer wrapper in `pkg/kafka` retries up to 3 times with exponential backoff (1s, 2s, 4s). After all retries, the message is published to a `dlq` topic with headers indicating the original error and source topic, and the offset is committed so the partition is not blocked (poison message).

### Validation in 3 layers
1. **Syntactic** (HTTP handler): validator/v10 tags check types, formats, ranges. Returns `422` with field-level details.
2. **Business** (service): rules like "enough balance", "same currency on both sides". Typed errors in `domain/errors.go`.
3. **DB constraints** (migrations): `CHECK (balance >= 0)`, `UNIQUE (idempotency_key)`, `CHECK (amount > 0)`, `FOREIGN KEY`. The last line of defense: even with a bug in the code, Postgres rejects the operation.

## Technical decisions and trade-offs

### Why Apache Kafka in KRaft mode
The challenge literally asks for Kafka. I use KRaft mode (no Zookeeper) because since Kafka 3.3+ it has been production-ready and removes all of Zookeeper's operational complexity. Much smaller footprint (~700MB vs ~1.5GB), faster startup, and only one process to operate.

### Why no ORM (pgx + raw SQL)
Queries are simple and few (5-6 per service). An ORM would add magic and complexity without adding value. pgx with raw SQL is more idiomatic in senior Go and reads directly. **It does not introduce vulnerabilities**: every query uses placeholders (`$1, $2`), never string concatenation, so SQL injection is impossible.

If this were a project with 30+ queries, I would consider `sqlc` (generates type-safe Go code from `.sql` files), which is the modern senior Go option. For this size it would be over-engineering.

### Why chi over Gin/Echo/Fiber
chi is the only top Go router that is **100% compatible with `net/http` standard**. That means any middleware in the Go ecosystem works without adapters. Gin/Echo have their own `Context` and break that compatibility. Fiber uses `fasthttp`, a different HTTP stack altogether. chi is what a senior Go dev picks when they want a no-magic library.

### Why a materialized view in llm-ms instead of HTTP-calling transactions-ms
Three options were possible:

| Option | Coupling | Resilience | Performance |
|---|---|---|---|
| Front passes context on every request | Low | High | Good |
| llm-ms does HTTP to transactions-ms | High | Low | Bad (1 extra hop) |
| **Local materialized view** ✅ | **Zero** | **Total** | **Excellent** |

The materialized view is the orthodox pattern in event-driven architectures: each service keeps the data it needs in its own storage, fed by events. Trade-off: small data duplication and eventual consistency (ms-level delay between transactions-ms and llm-ms, irrelevant for the chat).

### Sync vs Async — when each one

| Operation | Type | Why |
|---|---|---|
| Create client / account | HTTP sync | Immediate user action, single DB |
| Check balance | HTTP sync | Simple read |
| Start transfer | HTTP async (202) + Kafka | Coordination between 2 services, we want decoupling |
| Generate tx explanation | Kafka consumer (background) | Reaction to a business event, not user-initiated |
| Chat with LLM | HTTP sync with SSE streaming | User↔system interaction, not a business event |
| Read explanation | HTTP sync | Read from local cache |

**Rule**: use async/events when there is real decoupling between producers and consumers. Use sync when there is immediate request-response. **Don't force event-driven where it doesn't add value.**

### Stateless chat
LLMs are stateless by design. Continuity in the chat is achieved by sending the full message history on each request. In our case, the front keeps the `messages` array in React state and sends it whole to the backend. No session tables, no auth, no complexity. It is what ChatGPT does in non-logged-in interfaces.

### SSE instead of WebSockets for the chat
SSE (Server-Sent Events) is simpler than WebSockets for unidirectional server→client communication. It is plain HTTP with `Content-Type: text/event-stream`, no separate protocol. WebSockets would be overkill — the chat does not need simultaneous bidirectional communication, just streaming the LLM response.

**Cost**: streaming via the Claude API does NOT cost more than non-streaming. The same tokens are billed. Only the delivery method changes.

### Kafka UI instead of a custom "debug stream" endpoint
At first I considered making a `/debug/events` endpoint in each microservice so the front would show Kafka events live in its panel. I dropped it: Kafka UI (provectus) already exists, is free, professional, does that and much more without writing a single line of custom code. The front panel sticks to HTTP requests and SSE chunks; Kafka UI takes care of bus inspection.

## Repo structure

```
banking-platform-go/
├── go.work                       # Go workspace with the 4 modules
├── docker-compose.yml            # Brings up EVERYTHING with one command
├── .env.example
├── README.md                     # this file
│
├── pkg/                          # code shared across microservices
│   ├── events/                   # envelope, types, topics, payloads
│   ├── kafka/                    # producer + consumer with retry/DLQ
│   ├── outbox/                   # outbox-pattern worker
│   └── httpx/                    # HTTP helpers (validation, errors, middlewares)
│
├── services/
│   ├── accounts-ms/              # clients and accounts
│   │   ├── cmd/main.go           # composition root
│   │   ├── internal/
│   │   │   ├── config/           # env vars
│   │   │   ├── domain/           # entities + typed errors
│   │   │   ├── repo/             # Postgres access via pgx
│   │   │   ├── service/          # use cases + Kafka handler
│   │   │   └── http/             # chi handlers + router + middlewares
│   │   ├── migrations/           # *.up.sql / *.down.sql
│   │   └── Dockerfile
│   ├── transactions-ms/          # same shape
│   └── llm-ms/                   # same shape + internal explainer/ with Claude/Mock
│
├── web/                          # Next.js frontend
│   ├── app/                      # App Router (pages)
│   ├── components/               # ActivityPanel, TransactionChat
│   ├── lib/                      # api.ts (fetch wrapper) + activityStore.ts
│   └── Dockerfile
│
└── test/
    └── e2e/                      # e2e test (e2e build tag)
```

**Hexagonal lite**: each Go service has 4 layers (`domain` → `repo` → `service` → `http`). `domain` knows nothing about infrastructure. Each layer only knows the one below. `main.go` is the composition root: the only place where everything is wired together.

## Tests

### Unit tests

```bash
# from the repo root
cd services/accounts-ms && go test ./...
cd services/transactions-ms && go test ./...
cd services/llm-ms && go test ./...
cd pkg && go test ./...
```

They cover the core logic: state machines, balance validation, idempotency, explainer with deterministic mock, envelope round-trip.

### E2E test

Assumes the system is up:

```bash
docker compose up -d
go test -tags=e2e ./test/e2e/... -v
```

Covers two flows:

1. **Happy path**: create client → 2 accounts → deposit → transfer → verify final balances → verify the LLM generated the explanation.
2. **Rejected**: try to transfer more than available → verify the transaction ends in `REJECTED` with `rejection_code=insufficient_funds` → verify the LLM also generated the rejection explanation.

## What I would leave for v2

Things a production system would have but are out of scope for a challenge:

- **Auth and authorization**: today there is no JWT, OAuth, or anything. In production I would use JWT with an external IdP and gating on every endpoint.
- **Kafka ACLs**: today the isolation between microservices is by convention (each topic has a clear owner). In production I would set up SASL/SSL ACLs to enforce it at the broker level.
- **Metrics and tracing**: I would add Prometheus + Grafana + OpenTelemetry for distributed tracing across services. The RequestID I already propagate over HTTP would serve as a baseline.
- **Retry topics instead of in-process retries**: to avoid blocking the partition during long retries, I would publish to `retry.5s`, `retry.30s` topics in the Uber style.
- **CDC with Debezium**: the current outbox uses polling with a worker. Debezium would let me push rows into Kafka instantly via Postgres logical replication.
- **Persistent chat sessions**: today the chat is stateless on the backend. For authenticated users I would store conversations in a `chat_sessions` table so they can be resumed.
- **More unit tests with sqlmock or testcontainers**: today the unit tests cover the pure logic. I would add tests for the repos against a real ephemeral DB.
- **Multi-currency with conversion**: transfers today require the same currency. A v2 could perform real-time conversion with rates.
- **Richer auditing**: the outbox is already a change log, but I would add a separate `audit_log` table with who did what.

---

**Repository**: https://github.com/j0sehernan/banking-platform-go
