# retrievion

**retrievion** is a multi-tenant Retrieval-Augmented Generation (RAG) backend built with Go.

It provides asynchronous document ingestion (PDF and DOCX), hybrid semantic/keyword retrieval with optional API reranking, grounded LLM responses, citations, streaming chat, conversation persistence, tenant invitations, evaluation, and production-oriented observability.

> **The LLM should reason over retrieved application data, while the backend remains responsible for storage, retrieval, authorization, orchestration, and persistence.**

---

## Architecture

```text
                              ┌─────────────────┐
                              │     Client      │
                              └────────┬────────┘
                                       │
                                       ▼
                              ┌─────────────────┐
                              │    Go / Gin     │
                              │      API        │
                              └────────┬────────┘
                                       │
                    ┌──────────────────┴──────────────────┐
                    ▼                                     ▼
             Document API                            Chat API
                    │                                     │
                    ▼                                     ▼
             SHA-256 dedupe                         Conversation
                    │                                     │
                    ▼                                     ▼
             Backblaze B2                         Query embedding
                    │                                     │
                    ▼                                     ▼
       PostgreSQL + ingestion outbox              Hybrid Retrieval
                    │                              │        │
                    ▼                              ▼        ▼
               RabbitMQ                       pgvector     FTS
                    │                              └───┬────┘
                    ▼                                  ▼
            Ingestion Worker                          RRF
                    │                                  │
                    ▼                                  ▼
       Parse (PDF / DOCX)                  Reranker (Cohere or no-op)
       Chunking                                        │
       Embeddings                                      ▼
                    │                           Context budget
                    ▼                                  │
              PostgreSQL                               ▼
                                                Prompt builder
                                                       │
                                                       ▼
                                                    Gemini
                                                       │
                                        ┌──────────────┴──────────────┐
                                        ▼                             ▼
                                    Generate                       SSE Stream
                                        └──────────────┬──────────────┘
                                                       ▼
                                                  Citations
                                                       │
                                                       ▼
                                            messages + sources
```

### Tenant invitation flow

```text
 Tenant owner ── POST /api/v1/tenants/invitations
        │
        ▼
 Owner authorization → validate emails / roles / duplicates
        │
        ▼
 Generate token + SHA-256 token hash
        │
        ▼
 PostgreSQL transaction (all or none)
   ├── tenant_invitations (status = pending)
   └── outbox_messages (email job)
        │
        ▼
 202 Accepted to the client

 Outbox relay (polls every 2s)
   claims rows (FOR UPDATE SKIP LOCKED)
   → publishes to RabbitMQ (broker-confirmed)
   → deletes the row, or records the failure with exponential backoff

 RabbitMQ
   invitations.email         main queue
   invitations.email.retry   30s TTL, dead-letters back
   invitations.email.dead    parking lot after max attempts

 Invitation worker
   success            → ack
   transient failure  → retry queue (max 5 attempts)
   permanent failure  → dead queue
   success            → email with a link carrying the raw one-time token
```

For deeper architectural details, see [`docs/architecture.md`](docs/architecture.md).

---

## Core Features

### Document ingestion

* PDF and DOCX ingestion (file type detected from content, not the extension)
* Tenant-scoped SHA-256 deduplication
* Versioned documents, with one active version at a time
* Backblaze B2 object storage
* Asynchronous processing through RabbitMQ, published from a transactional outbox
* Stale-job recovery: jobs stuck in `processing` for more than 5 minutes are re-queued automatically
* Dead-letter queue for jobs that fail
* Page-aware parent/child chunking
* Gemini embeddings, in batches, with retries on transient errors
* Transactional chunk persistence

### Hybrid retrieval

```text
User question
     │
     ├───────────────┐
     ▼               ▼
Vector Search    Keyword Search
pgvector         PostgreSQL FTS
(limit 20)       (limit 20)
     │               │
     └───────┬───────┘
             ▼
        RRF Fusion (K = 60)
             │
             ▼
   Reranker (configurable)
   falls back to RRF order on error
             │
             ▼
   Top 10 chunks
             │
             ▼
   Token-bounded context
```

Semantic search uses **pgvector cosine distance** with an HNSW index. Keyword search uses PostgreSQL English full-text search with a GIN index. Results are merged using **Reciprocal Rank Fusion (RRF)** with `K=60`.

#### Reranking

The reranker is selected with `RERANKER_PROVIDER`:

| Value | Behavior |
| --- | --- |
| empty, `noop`, `none` | Keeps the RRF order (default) |
| `cohere` | Reorders candidates with Cohere's Rerank API |

The Cohere reranker validates inputs, retries 5xx and 429 responses up to 2 times, and returns the original chunks unchanged, so RRF/vector/keyword scores are preserved. If reranking fails for any reason, retrieval logs a warning and continues with the RRF order, so a reranker outage does not fail the chat request.

Cohere trial keys are limited to roughly 10 rerank calls per minute and 1,000 calls per month, and are not intended for production use.

### RAG chat

```text
Authenticate
    ↓
Validate conversation ownership
    ↓
Persist user message
    ↓
Embed question
    ↓
Hybrid retrieval (+ reranking)
    ↓
Build bounded context
    ↓
Build grounded prompt
    ↓
Gemini generation
    ↓
Resolve [1], [2], ... citations
    ↓
Persist assistant message and message sources
    ↓
Return response
```

The same pipeline supports streaming through **Server-Sent Events (SSE)**. Streaming events include `message_start`, `token`, `citation`, `message_complete`, and `error`.

### Tenant invitations

* Owner-only invitations, with multiple users per request
* Invitable roles: `admin`, `member`, `viewer`
* Batch invitations stored atomically: all succeed or none do
* Emails normalized to lowercase and validated; duplicate emails in one request are rejected
* One pending invitation per tenant and email, enforced by a partial unique index
* Expired-but-pending invitations are revoked automatically so re-inviting is not blocked
* 24-hour invitation lifetime
* **Transactional outbox**: the invitation and its email job are committed together, then relayed to RabbitMQ with broker confirmation, so a RabbitMQ outage delays emails but does not lose them
* Asynchronous delivery with a 30-second retry queue (up to 5 attempts) and a dead-letter queue

The raw token exists only in the invitation email link. The database stores the SHA-256 digest only, so a database leak does not expose usable invitation links.

---

## Grounded Responses and Citations

Retrieved document content is treated as **untrusted context** by the prompt builder. The model is instructed to answer using the retrieved context, ignore instructions contained in retrieved documents, say when the context is insufficient, and cite sources as `[1]`, `[2]`, and so on.

After generation, the citation service maps those references back to retrieved chunks (chunk → document → page → section), and the message/source relationships are persisted in PostgreSQL.

---

## Document Processing

```text
Document (PDF / DOCX)
 │
 ├── Backblaze B2
 └── PostgreSQL metadata
       └── document version
              ├── Parent chunks
              └── Child chunks
                    └── Embeddings
```

| Setting             |      Value |
| ------------------- | ---------: |
| Parent unit         |   PDF page |
| Maximum child size  | 700 tokens |
| Minimum child size  |  50 tokens |
| Paragraph overlap   |        15% |
| Embedding dimension |       1536 |

Only child chunks receive embeddings.

---

## Storage

PostgreSQL is the primary system of record. It stores tenants, users and memberships, invitations, documents and versions, chunks, embeddings (`vector(1536)` with an HNSW cosine index), conversations, messages, message sources, ingestion jobs, the ingestion and invitation outboxes, LLM telemetry, evaluation data, and service clients.

**Backblaze B2** stores the original files through its S3-compatible API, with presigned download URLs.

**Redis** holds opaque auth tokens, refresh/access token state, OTPs, counters and budget tracking, and other short-lived state.

**RabbitMQ** carries ingestion jobs and invitation emails. The ingestion queue is durable. When an ingestion job fails, it is routed to a dead-letter queue and is **not** retried automatically. Jobs that are stuck in `processing` are handled by the recovery worker instead.

| Queue                     | Purpose                                          |
| ------------------------- | ------------------------------------------------ |
| `invitations.email`       | Pending invitation emails                        |
| `invitations.email.retry` | Delay queue (30s TTL), dead-letters back to main |
| `invitations.email.dead`  | Messages that failed permanently or hit 5 tries  |

---

## Multi-Tenancy

Users are global identities and can belong to multiple tenants through `tenant_members`. Tenant identifiers are carried through the authenticated request context and used to scope data.

* Conversation access is scoped to `user_id + tenant_id`.
* Retrieval is scoped to the **tenant**. Every member of a tenant can retrieve from all of that tenant's active documents, regardless of role or who uploaded them. Per-user and per-document permissions are not implemented.

---

## Authentication

The API uses opaque tokens rather than exposing JWTs to clients.

```text
Client ── Bearer opaque token ──► Redis lookup ──► short-lived Ed25519 JWT ──► handlers
```

* bcrypt password hashing
* Opaque access tokens (24 hours) and refresh tokens (7 days), stored in Redis and revocable
* Service clients can authenticate with `client_id + client_secret` (bcrypt-protected)

---

## LLM Integration

The LLM layer integrates with Gemini through a dedicated HTTP client and supports normal, structured, and streaming generation, provider error classification, transient retries, configurable token/cost pricing, and token and latency tracking.

* Normal generation retries transient failures with exponential backoff and jitter.
* Streaming requests are not retried once tokens have started reaching the client.

---

## Evaluation

retrievion includes a pipeline for measuring retrieval quality and answer groundedness. Runs and results are stored in PostgreSQL.

**Retrieval metrics:** Precision@K, Recall@K, HitRate@K, NDCG@K. Evaluation cases are scoped to the selected tenant; cases with a `NULL` tenant ID are treated as global and excluded from tenant-scoped runs.

**Faithfulness:** generated answers are scored from 0.0 to 1.0 by a separate LLM judge, with a pass threshold of `0.7`. Both the score and the judge's reasoning are stored.

```bash
make eval-setup   # create or reuse the eval tenant, user and fixtures
make eval         # run the default dataset (resume-smoke-fixture-v1, K=10)

EVAL_DATASET=resume-smoke-fixture-v1 EVAL_K=10 make eval
EVAL_TENANT_ID=<tenant-uuid> EVAL_USER_ID=<user-uuid> make eval
```

Prerequisites: PostgreSQL and the other infrastructure running, valid Gemini credentials, and the API started at least once so migrations are applied. Gemini access is needed when fixture embeddings must be generated.

The evaluator builds its retrieval service from the same reranker configuration as the API (`RERANKER_PROVIDER`). If a rerank call fails or is rate-limited (for example on a Cohere trial key), retrieval falls back to the RRF order for that case, so a run can mix reranked and non-reranked results. Use a production key, or pace the run, before comparing results across runs.

A run may be incomplete if individual cases fail or Gemini rejects requests (rate limits and exhausted daily quotas are common). Check the evaluated and failed case counts and rerun before using results to support quality claims. Automated CI evaluation gates are not implemented.

---

## Observability

* **Prometheus:** LLM request count, errors, input/output tokens, estimated cost, latency, time to first token, retrieval latency, retrieval hit rate, evaluation pass rate, groundedness score.
* **OpenTelemetry:** traces exported over OTLP/gRPC, with LLM spans (model, provider, tokens, latency, finish reason) and retrieval spans (chunk IDs, scores, latency).
* **Sentry:** error reporting and request context through Gin middleware.

---

## API and Worker

```text
┌───────────────────────┐        ┌───────────────────────┐
│          API          │        │        Worker         │
│ HTTP / Gin            │        │ Document parsing      │
│ Authentication        │ ─────► │ Chunking              │
│ Documents             │RabbitMQ│ Embeddings            │
│ Conversations / Chat  │        │ Chunk persistence     │
│ Tenant invitations    │        │ Stale-job recovery    │
│ Outbox relays         │        │ Invitation emails     │
└───────────────────────┘        └───────────────────────┘
```

The API runs database migrations at startup. The worker consumes ingestion and invitation jobs concurrently and drains in-flight work on shutdown.

### HTTP routes

| Area | Routes |
| --- | --- |
| Health | `GET /healthz` |
| Auth | `POST /register`, `/login`, `/refresh`, `/logout`, `/service/token`, `/select-tenant` |
| Documents | `POST`, `GET` (list), `GET /:id`, `DELETE /:id` |
| Chat | `POST /conversations`, `GET /conversations/:id`, `POST /conversations/messages`, `POST /conversations/:id/stream` |
| Invitations | `POST /tenants/invitations` |

---

## Technology Stack

| Component        | Technology                        |
| ---------------- | --------------------------------- |
| Language         | Go                                |
| HTTP             | Gin                               |
| Database         | PostgreSQL 16                     |
| Vector search    | pgvector (HNSW)                   |
| Keyword search   | PostgreSQL FTS + GIN              |
| Reranking        | Cohere Rerank (optional)          |
| Object storage   | Backblaze B2                      |
| Queue            | RabbitMQ                          |
| Cache/state      | Redis                             |
| Embeddings       | Gemini                            |
| LLM              | Gemini                            |
| Streaming        | Server-Sent Events                |
| Authentication   | Redis opaque tokens + Ed25519 JWT |
| Metrics          | Prometheus                        |
| Tracing          | OpenTelemetry                     |
| Error monitoring | Sentry                            |
| Containers       | Docker / Docker Compose           |

---

## Local Development

### Prerequisites

Go, Docker, Docker Compose, and a Gemini API key.

### Setup

```bash
git clone https://github.com/MTariq99/retrievion.git
cd retrievion
cp env.example .env
```

Set the database, Redis, RabbitMQ, Gemini, and Backblaze B2 configuration. At minimum, configure the database and Gemini credentials.

### Configuration notes

| Variable | Notes |
| --- | --- |
| `OUTBOX_MESSAGES_BATCH` | **Required to be greater than 0.** Controls how many outbox rows the invitation relay claims per cycle. If it is unset, no invitation emails are relayed. Recommended: `100`. |
| `RERANKER_PROVIDER` | `cohere`, or empty/`noop` to disable reranking. |
| `COHERE_RERANKER_API_KEY` | Required when the provider is `cohere`. |
| `COHERE_RERANK_MODEL` | A single Cohere model ID, such as `rerank-v5.0-fast`. Defaults to `rerank-v5.0-fast` when empty. |
| `COHERE_RERANK_TIMEOUT` | Whole seconds, such as `10`. Values below 10 are raised to 10. |

Model IDs available to your key can be listed with:

```bash
curl -s "https://api.cohere.com/v1/models?endpoint=rerank" \
  -H "Authorization: Bearer $COHERE_RERANKER_API_KEY"
```

### Run

```bash
docker compose -f deployments/docker-compose.yaml up -d postgres redis rabbitmq
go run ./cmd/api        # listens on :8080 by default
go run ./cmd/worker     # in a second terminal
```

Or run everything with Docker Compose:

```bash
docker compose -f deployments/docker-compose.yaml up --build
```

---

## Project Structure

```text
retrievion/
├── cmd/
│   ├── api/
│   ├── worker/
│   ├── eval/
│   └── eval-setup/
├── deployments/docker-compose.yaml
├── docs/architecture.md
├── internal/
│   ├── auth/  chat/  citation/  config/  conversation/
│   ├── document/  embedding/  evaluation/  filetype/
│   ├── ingestion/  llm/  middleware/  notifications/
│   ├── prompt/  retrieval/  storage/  tenants/  tokenizer/
├── migrations/
├── models/
├── pkg/
├── utils/
├── Dockerfile
└── README.md
```

Tests live next to the code as `*_test.go` files.

---

## Engineering Decisions

**PostgreSQL + pgvector.** Metadata, chunks, embeddings, conversations, and citations live in one transactional system, which avoids running a separate vector database.

**Hybrid retrieval.** Semantic search handles conceptual similarity; keyword search handles exact terms, names, and identifiers. RRF combines the two rankings simply.

**Asynchronous ingestion.** Parsing and embedding are expensive, so they run behind RabbitMQ and keep upload requests short.

**Outboxes for ingestion and invitations.** Publishing to a broker inside a request can fail after the database commit. Writing the job to an outbox table in the same transaction, then relaying it, prevents lost jobs.

**Asynchronous invitation emails.** Sending email inside the request took 3 to 12 seconds. The invitation is committed first, the API returns `202 Accepted`, and the worker handles delivery with retries.

**Hashed invitation tokens.** The token is a bearer credential, so only its SHA-256 digest is stored.

**Opaque tokens.** Clients never handle the internal JWT. Redis allows revocation and introspection while the application still uses short-lived signed JWTs internally.

---

## Current Limitations

**Not implemented:**

* Resend or revoke invitation endpoints.
* Automatic retries for ingestion jobs that fail. Only jobs stuck in `processing` are recovered.
* Query rewriting.
* Fine-grained retrieval permissions. Retrieval is scoped to the tenant, not to individual users or documents.
* Automated CI/CD evaluation gating.
* Kubernetes deployment manifests.



---

## License

See the repository license for usage and distribution terms.