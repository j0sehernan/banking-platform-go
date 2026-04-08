# banking-platform-go

Plataforma bancaria simplificada construida con **Go + Apache Kafka + Claude (LLM)** para resolver el challenge técnico de la entidad financiera.

Tres microservicios desacoplados se comunican vía un bus de eventos. Un cuarto componente (LLM) consume los eventos del bus y genera explicaciones en lenguaje natural. Hay también un frontend Next.js que sirve como herramienta de demo: además de una UI funcional, incluye un panel lateral que muestra en vivo todos los HTTP requests/responses, y un chat con el LLM con streaming SSE acotado a una transacción específica.

## Tabla de contenidos

- [Arquitectura general](#arquitectura-general)
- [Stack técnico](#stack-técnico)
- [Cómo levantar todo (un solo comando)](#cómo-levantar-todo-un-solo-comando)
- [Endpoints disponibles](#endpoints-disponibles)
- [Flujo completo de una transferencia](#flujo-completo-de-una-transferencia)
- [Rol del microservicio LLM](#rol-del-microservicio-llm)
- [Patrones implementados](#patrones-implementados)
- [Decisiones técnicas y trade-offs](#decisiones-técnicas-y-trade-offs)
- [Estructura del repo](#estructura-del-repo)
- [Tests](#tests)
- [Qué dejaría para una v2](#qué-dejaría-para-una-v2)

## Arquitectura general

```
                                  ┌──────────────────────────────┐
                                  │      Apache Kafka 3.9        │
                                  │      (KRaft, sin ZK)         │
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
   │  · clientes        │              │  · deposit         │             │  · explanation     │
   │  · cuentas         │              │  · withdraw        │             │  · chat (SSE)      │
   │  · saldo no neg.   │              │  · transfer        │             │  · materialized    │
   │  · outbox + idemp. │              │  · saga orchestr.  │             │    view de tx      │
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
   │  · listado/creación de cuentas         · panel lateral de actividad técnica      │
   │  · creación de transacciones           · chat scoped a una tx con SSE streaming  │
   └──────────────────────────────────────────────────────────────────────────────────┘
```

**Principios:**

- Cada microservicio tiene **su propia base de datos** (no comparten tablas).
- La comunicación entre microservicios es **siempre vía Kafka**, nunca HTTP directo.
- Cada microservicio publica eventos a sus topics y consume de los topics que le interesan.
- El frontend solo habla HTTP con cada microservicio (CORS habilitado).

## Stack técnico

| Capa | Librería | Por qué |
|---|---|---|
| Lenguaje backend | Go 1.23 | Lo que pide el enunciado |
| Bus de eventos | Apache Kafka 3.9 (KRaft mode) | Lo que pide el enunciado, sin Zookeeper porque ya está deprecated |
| HTTP router | `go-chi/chi/v5` | Minimalista, idiomático Go, compatible con `net/http` estándar |
| Postgres client | `jackc/pgx/v5` | Sin ORM, queries simples y type-safe |
| Migraciones | `golang-migrate/migrate/v4` | Estándar de facto, archivos SQL versionados |
| Kafka client | `segmentio/kafka-go` | Go puro sin CGO, fácil de buildear en Docker |
| Validación | `go-playground/validator/v10` | Tags declarativos en los DTOs |
| Decimal | `shopspring/decimal` | Crítico en banca para no perder precisión (NUMERIC en PG) |
| Config | `caarlos0/env/v11` | Carga de env vars a struct con tags |
| LLM | `anthropics/anthropic-sdk-go` | SDK oficial de Claude |
| Logging | stdlib `log/slog` | Structured logging built-in (Go 1.21+) |
| Frontend | Next.js 15 + React 19 + Tailwind | Stack moderno, App Router |
| Estado del front | Zustand | Store global liviano (~2KB), sin boilerplate |
| Observabilidad del bus | Kafka UI (provectus) | http://localhost:8090, gratis y profesional |

## Cómo levantar todo (un solo comando)

**Prerequisitos:** Docker y Docker Compose v2.

```bash
git clone https://github.com/j0sehernan/banking-platform-go.git
cd banking-platform-go

# Opcional: si tenés ANTHROPIC_API_KEY, copiá .env.example a .env y editalo.
# Si no, llm-ms arranca con MockExplainer y todo sigue funcionando.
cp .env.example .env

# Levantar todo
docker compose up
```

A los ~30 segundos vas a tener:

| URL | Qué es |
|---|---|
| http://localhost:3000 | Frontend Next.js |
| http://localhost:8090 | **Kafka UI** (inspeccioná topics, mensajes, consumer groups) |
| http://localhost:8081 | accounts-ms HTTP API |
| http://localhost:8082 | transactions-ms HTTP API |
| http://localhost:8083 | llm-ms HTTP API |
| `localhost:5433/5434/5435` | Postgres (accounts/transactions/llm), `psql -h localhost -p 5433 -U accounts -d accounts` |

Para limpiar todo (incluyendo volúmenes):

```bash
docker compose down -v
```

## Endpoints disponibles

### accounts-ms (`:8081`)

| Método | Path | Body |
|---|---|---|
| `POST` | `/clients` | `{ "name": "...", "email": "..." }` |
| `GET` | `/clients/{id}` | — |
| `POST` | `/accounts` | `{ "client_id": "uuid", "currency": "USD" }` |
| `GET` | `/accounts` | — |
| `GET` | `/accounts/{id}` | — |
| `GET` | `/accounts/{id}/balance` | — |

### transactions-ms (`:8082`)

| Método | Path | Body |
|---|---|---|
| `POST` | `/transactions/deposit` | `{ "to_account_id", "amount", "currency", "idempotency_key" }` |
| `POST` | `/transactions/withdraw` | `{ "from_account_id", "amount", "currency", "idempotency_key" }` |
| `POST` | `/transactions/transfer` | `{ "from_account_id", "to_account_id", "amount", "currency", "idempotency_key" }` |
| `GET` | `/transactions` | — |
| `GET` | `/transactions/{id}` | — |
| `GET` | `/accounts/{id}/transactions` | — |

### llm-ms (`:8083`)

| Método | Path | Body |
|---|---|---|
| `GET` | `/transactions/{id}/explanation` | — |
| `POST` | `/chat` | `{ "tx_id", "messages": [{"role","content"}, ...] }` (responde SSE stream) |

### Ejemplo manual con curl

```bash
# 1. Crear cliente
curl -X POST localhost:8081/clients \
  -H "Content-Type: application/json" \
  -d '{"name":"Ana Perez","email":"ana@example.com"}'

# Guardá el id devuelto, lo usás abajo (ANA_ID)

# 2. Crear cuenta
curl -X POST localhost:8081/accounts \
  -H "Content-Type: application/json" \
  -d "{\"client_id\":\"$ANA_ID\",\"currency\":\"USD\"}"

# 3. Depositar
curl -X POST localhost:8082/transactions/deposit \
  -H "Content-Type: application/json" \
  -d "{\"to_account_id\":\"$ACC_ID\",\"amount\":\"1000\",\"currency\":\"USD\",\"idempotency_key\":\"dep-001\"}"

# 4. Ver saldo (debería ser 1000 después de unos segundos)
curl localhost:8081/accounts/$ACC_ID/balance

# 5. Ver explicación generada por Claude
curl localhost:8083/transactions/$TX_ID/explanation
```

## Flujo completo de una transferencia

Veamos paso a paso qué pasa cuando un usuario hace una transferencia de Ana → Luis por 300 USD.

```
1. POST /transactions/transfer
   ─────────────────────────────
   El front llama a transactions-ms con el body de la transferencia.

2. transactions-ms (en una sola tx PG):
   ────────────────────────────────────
   BEGIN
     INSERT transactions (id, status='PENDING', ...)
     INSERT outbox (topic='transactions.commands', payload=TransactionRequested)
   COMMIT
   → 202 Accepted al cliente con { id, status: PENDING }

3. outbox-worker de transactions-ms (background):
   ──────────────────────────────────────────────
   Lee filas pendientes de outbox cada 500ms con FOR UPDATE SKIP LOCKED.
   Publica TransactionRequested al topic transactions.commands.

4. accounts-ms consume de transactions.commands:
   ─────────────────────────────────────────────
   Verifica idempotencia (INSERT processed_events ON CONFLICT DO NOTHING).
   BEGIN
     UPDATE accounts SET balance = balance - 300
       WHERE id = AnaAcc AND balance >= 300   ← atómico, sin saldo negativo posible
     UPDATE accounts SET balance = balance + 300 WHERE id = LuisAcc
     INSERT outbox (topic='accounts.tx-results', payload=AccountsTransferApplied)
     INSERT outbox (topic='accounts.events', payload=BalanceUpdated × 2)
   COMMIT

   Si el saldo era insuficiente, el primer UPDATE no afecta filas.
   accounts-ms hace ROLLBACK y publica AccountsTransferFailed con razón.

5. outbox-worker de accounts-ms publica al bus.

6. transactions-ms consume de accounts.tx-results:
   ──────────────────────────────────────────────
   Verifica idempotencia.
   BEGIN
     UPDATE transactions SET status = 'COMPLETED' (o REJECTED) WHERE id = txID
     INSERT outbox (topic='transactions.events', payload=TransactionCompleted/Rejected)
   COMMIT

7. outbox-worker de transactions-ms publica al bus.

8. llm-ms consume de transactions.events:
   ──────────────────────────────────────
   Verifica idempotencia.
   UPSERT en transactions_view (mantiene su copia local de transacciones).
   En background (goroutine): llama a Claude → guarda explicación.

9. Front (en paralelo, polling):
   ─────────────────────────────
   GET /transactions/{id} → status: COMPLETED
   GET /transactions/{id}/explanation → texto generado
```

Esto es una **saga orquestada** con `transactions-ms` como orquestador implícito: él inicia (paso 1), recibe el resultado de accounts (paso 6), y emite el evento final (paso 7). El "ciclo del saga" se cierra cuando transactions-ms recibe la respuesta de accounts y puede marcar el estado final.

**Compensación**: como el débito y el crédito ocurren en una sola transacción Postgres en accounts-ms, si el crédito al destino fallara, el débito al origen se revierte automáticamente con `ROLLBACK`. La saga es simple porque la atomicidad local elimina la necesidad de pasos compensatorios complejos.

## Rol del microservicio LLM

`llm-ms` no tiene lógica bancaria propia. Su rol es **interpretar información** y generar texto en lenguaje natural. Cumple con el requisito del enunciado de "explicar una transacción bancaria".

Tiene tres responsabilidades:

1. **Mantener una vista materializada de transacciones (read model)**: consume `transactions.events` desde Kafka y mantiene una copia local en `transactions_view`. Esto le permite responder preguntas sobre cualquier transacción sin tocar a transactions-ms vía HTTP.

2. **Generar explicaciones automáticas**: cada vez que llega un `TransactionCompleted` o `TransactionRejected`, llm-ms llama a Claude con el contexto y guarda el texto en `transaction_explanations`. Cuando el front pide la explicación, se devuelve cacheada (no se llama a Claude cada vez).

3. **Chat scoped a una transacción**: el endpoint `POST /chat` recibe el `tx_id` y el historial de mensajes del front, lee el contexto de la transacción de su vista local, construye un system prompt con context grounding y reglas de scope estrictas, y devuelve un stream SSE con la respuesta del LLM. El usuario solo puede preguntar sobre **esa** transacción específica.

**Adapter pattern para el LLM**: la interface `Explainer` tiene dos implementaciones:

- `ClaudeExplainer`: usa el SDK oficial de Anthropic, modelo `claude-haiku-4-5` (rápido, barato).
- `MockExplainer`: respuestas template deterministas. Se usa automáticamente si `ANTHROPIC_API_KEY` está vacío. Permite que el sistema funcione completo en local sin credenciales.

**Por qué el chat NO pasa por Kafka**: el chat es comunicación sincrónica usuario↔llm-ms↔Claude, no hay eventos de negocio que otros servicios necesiten procesar. Mezclar Kafka acá sería over-engineering (ver sección "Sync vs Async" abajo).

## Patrones implementados

### Outbox pattern
Garantiza atomicidad entre cambios de estado en la DB y la publicación de eventos a Kafka, sin two-phase commit. El cambio de estado y la fila del outbox se insertan en la misma transacción Postgres. Un worker en background lee con `FOR UPDATE SKIP LOCKED` y publica a Kafka. Si Kafka está caído, los eventos se acumulan en outbox y se publican cuando vuelve.

Sin esto, un crash justo después de un commit pero antes de publicar perdería el evento → estado inconsistente entre microservicios. **Inaceptable en banca.**

### Inbox pattern (idempotencia de consumers)
Kafka garantiza at-least-once delivery, lo que significa que el mismo evento puede llegar dos o más veces (rebalance, restart, commit que falla). Si no manejamos idempotencia, un re-delivery causaría doble débito en una cuenta.

Cada consumer hace `INSERT INTO processed_events (event_id) ON CONFLICT DO NOTHING RETURNING event_id`. Si no devuelve fila, el evento ya fue procesado y se hace skip.

### Saga orquestada
La transferencia es una operación que cruza dos microservicios (transactions-ms y accounts-ms), por lo que no podemos hacerla en una sola transacción ACID. La modelamos como una saga: pasos secuenciales coordinados por eventos, con `transactions-ms` como orquestador implícito.

### Materialized view / Read model
`llm-ms` no tiene "su" data de transacciones — pero la necesita para responder al chat. Mantiene una copia local (`transactions_view`) alimentada por los eventos de Kafka. Esto elimina el acoplamiento HTTP entre llm-ms y transactions-ms, y le da resiliencia: aunque transactions-ms esté caído, el chat sigue funcionando.

### Retries con backoff exponencial + DLQ
El wrapper de consumer en `pkg/kafka` reintenta hasta 3 veces con backoff exponencial (1s, 2s, 4s). Si después de los retries el mensaje sigue fallando, se publica a un topic `dlq` con headers que indican el error original y el topic de origen, y se commitea el offset para no bloquear la partición (poison message).

### Validación en 3 capas
1. **Sintáctica** (handler HTTP): tags de validator/v10 chequean tipos, formatos, rangos. Devuelve `422` con detalles por campo.
2. **Negocio** (service): reglas como "saldo suficiente", "misma moneda en transferencia". Errores tipados en `domain/errors.go`.
3. **DB constraints** (migrations): `CHECK (balance >= 0)`, `UNIQUE (idempotency_key)`, `CHECK (amount > 0)`, `FOREIGN KEY`. Es la última línea de defensa: aunque haya un bug en el código, Postgres rechaza la operación.

## Decisiones técnicas y trade-offs

### Por qué Apache Kafka en KRaft mode
El enunciado pide Kafka literalmente. Uso KRaft mode (sin Zookeeper) porque desde Kafka 3.3+ es production-ready y se elimina toda la complejidad operativa de Zookeeper. Footprint mucho menor (~700MB vs ~1.5GB), arranque más rápido, y un solo proceso a operar.

### Por qué sin ORM (pgx + SQL crudo)
Las queries son simples y pocas (5-6 por servicio). Un ORM agregaría magia y complejidad sin aportar valor. pgx con SQL crudo es más idiomático en Go senior y se lee directamente. **No introduce vulnerabilidades**: todas las queries usan placeholders (`$1, $2`), nunca string concatenation, así que SQL injection es imposible.

Si fuera un proyecto con 30+ queries, evaluaría `sqlc` (genera código Go type-safe desde archivos `.sql`), que es la opción moderna en Go senior. Para este tamaño es over-engineering.

### Por qué chi sobre Gin/Echo/Fiber
chi es el único router del top de Go que es **100% compatible con `net/http` estándar**. Eso significa que cualquier middleware del ecosistema Go funciona sin adaptadores. Gin/Echo tienen su propio `Context` y rompen esa compatibilidad. Fiber usa `fasthttp` que es otro stack HTTP completamente. chi es lo que un dev Go senior elige cuando quiere una librería sin magia.

### Por qué Materialized View en llm-ms en vez de llamar HTTP a transactions-ms
Tres opciones eran posibles:

| Opción | Acoplamiento | Resiliencia | Performance |
|---|---|---|---|
| Front pasa contexto en cada request | Bajo | Alta | Buena |
| llm-ms hace HTTP a transactions-ms | Alto | Baja | Mala (1 hop extra) |
| **Materialized view local** ✅ | **Cero** | **Total** | **Excelente** |

El materialized view es el patrón ortodoxo en arquitecturas event-driven: cada servicio mantiene los datos que necesita en su propio storage, alimentado por eventos. Trade-off: pequeña duplicación de datos y eventual consistency (delay de ms entre transactions-ms y llm-ms, irrelevante para el chat).

### Sync vs Async — cuándo cada uno

| Operación | Tipo | Por qué |
|---|---|---|
| Crear cliente / cuenta | HTTP sync | Acción inmediata del usuario, una sola DB |
| Consultar saldo | HTTP sync | Lectura simple |
| Iniciar transferencia | HTTP async (202) + Kafka | Coordinación entre 2 ms, queremos desacoplar |
| Generar explicación de tx | Kafka consumer (background) | Reacción a evento de negocio, no la pide el usuario |
| Chat con LLM | HTTP sync con SSE streaming | Interacción usuario↔sistema, no es un evento de negocio |
| Ver explicación | HTTP sync | Lectura del cache local |

**Regla**: usar async/eventos cuando hay desacoplamiento real entre productores y consumidores. Usar sync cuando hay request-response inmediato. **No forzar event-driven donde no aporta.**

### Stateless chat
Los LLMs son stateless por diseño. La continuidad del chat se logra enviando el historial completo en cada request. En nuestro caso, el front mantiene el array `messages` en React state y lo manda completo al backend. Sin tablas de sesiones, sin auth, sin complejidad. Es lo que hace ChatGPT en interfaces sin login.

### SSE en vez de WebSockets para el chat
SSE (Server-Sent Events) es más simple que WebSockets para comunicación unidireccional servidor→cliente. Es HTTP normal con `Content-Type: text/event-stream`, sin protocolo aparte. WebSockets sería overkill — el chat no necesita comunicación bidireccional simultánea, solo streaming de la respuesta del LLM.

**Costo**: el streaming de Claude API NO cuesta más que el non-streaming. Los tokens facturados son los mismos. Solo cambia cómo se entregan.

### Kafka UI en vez de endpoint custom de "debug stream"
Inicialmente pensé en hacer un endpoint `/debug/events` en cada microservicio para que el front mostrara los eventos Kafka en vivo en su panel. Lo descarté: ya existe Kafka UI (provectus), gratis, profesional, hace eso y mucho más sin escribir una sola línea de código custom. El panel del front se queda con HTTP requests y SSE chunks, Kafka UI se queda con la inspección del bus.

## Estructura del repo

```
banking-platform-go/
├── go.work                       # Go workspace con los 4 módulos
├── docker-compose.yml            # Levanta TODO con un solo comando
├── .env.example
├── README.md                     # este archivo
│
├── pkg/                          # código compartido entre microservicios
│   ├── events/                   # envelope, tipos, topics, payloads
│   ├── kafka/                    # producer + consumer con retry/DLQ
│   ├── outbox/                   # worker del patrón outbox
│   └── httpx/                    # helpers HTTP (validación, errores, middlewares)
│
├── services/
│   ├── accounts-ms/              # clientes y cuentas
│   │   ├── cmd/main.go           # composition root
│   │   ├── internal/
│   │   │   ├── config/           # env vars
│   │   │   ├── domain/           # entidades + errores tipados
│   │   │   ├── repo/             # acceso a Postgres con pgx
│   │   │   ├── service/          # casos de uso + handler de Kafka
│   │   │   └── http/             # handlers chi + router + middlewares
│   │   ├── migrations/           # *.up.sql / *.down.sql
│   │   └── Dockerfile
│   ├── transactions-ms/          # idem
│   └── llm-ms/                   # idem + interno explainer/ con Claude/Mock
│
├── web/                          # frontend Next.js
│   ├── app/                      # App Router (pages)
│   ├── components/               # ActivityPanel, TransactionChat
│   ├── lib/                      # api.ts (fetch wrapper) + activityStore.ts
│   └── Dockerfile
│
└── test/
    └── e2e/                      # test e2e (build tag e2e)
```

**Hexagonal lite**: cada servicio Go tiene 4 capas (`domain` → `repo` → `service` → `http`). `domain` no conoce nada de infraestructura. Cada capa solo conoce a la de abajo. `main.go` es el composition root: el único lugar donde se "arma" todo.

## Tests

### Unit tests

```bash
# desde la raíz
cd services/accounts-ms && go test ./...
cd services/transactions-ms && go test ./...
cd services/llm-ms && go test ./...
cd pkg && go test ./...
```

Cubren la lógica core: máquinas de estados, validaciones de saldo, idempotencia, explainer con mock determinista, round-trip de envelopes.

### Test E2E

Asume que el sistema está corriendo:

```bash
docker compose up -d
go test -tags=e2e ./test/e2e/... -v
```

Cubre dos flujos:

1. **Happy path**: crear cliente → 2 cuentas → depositar → transferir → verificar saldos finales → verificar que el LLM generó la explicación.
2. **Rejected**: intentar transferir más de lo que hay → verificar que la transacción queda en `REJECTED` con `rejection_code=insufficient_funds` → verificar que el LLM también generó la explicación del rechazo.

## Qué dejaría para una v2

Cosas que un sistema en producción tendría pero que están fuera de scope para un challenge:

- **Auth y autorización**: hoy no hay JWT, OAuth, ni nada. En producción usaría JWT con un IdP externo y gating en cada endpoint.
- **ACLs en Kafka**: hoy el aislamiento entre microservicios es por convención (cada topic tiene un dueño claro). En producción configuraría ACLs SASL/SSL para enforcar a nivel de broker.
- **Métricas y tracing**: agregaría Prometheus + Grafana + OpenTelemetry para distributed tracing entre microservicios. El RequestID que ya propago en cada HTTP serviría como base.
- **Retry topics en vez de in-process retries**: para no bloquear la partición durante reintentos largos, publicaría a topics `retry.5s`, `retry.30s`, etc., al estilo Uber.
- **CDC con Debezium**: el outbox actual usa polling con worker. Debezium permitiría empujar las filas a Kafka instantáneamente vía replicación lógica de Postgres.
- **Persistencia de sesiones de chat**: hoy el chat es stateless en el backend. Para usuarios autenticados, guardaría las conversaciones en una tabla `chat_sessions` para que puedan retomarse.
- **Más unit tests con sqlmock o testcontainers**: hoy los unit tests cubren la lógica pura. Agregaría tests de los repos contra una DB real efímera.
- **Multi-currency con conversión**: las transferencias hoy requieren misma moneda. Una v2 podría hacer conversión con rates en tiempo real.
- **Auditoría más rica**: el outbox ya es un log de cambios, pero agregaría una tabla `audit_log` separada con quién hizo cada cosa.

---

**Repositorio**: https://github.com/j0sehernan/banking-platform-go
