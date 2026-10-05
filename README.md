# retrievion

**retrievion** is a multi-tenant Retrieval-Augmented Generation (RAG) backend built with Go.

It provides asynchronous PDF ingestion, hybrid semantic/keyword retrieval, grounded LLM responses, citations, streaming chat, conversation persistence, evaluation, and production-oriented observability.

The project is designed around a simple principle:

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
                    │                                     │
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
             PostgreSQL                        Hybrid Retrieval
                    │                              │        │
                    ▼                              ▼        ▼
               RabbitMQ                       pgvector     FTS
                    │                              │        │
                    ▼                              └───┬────┘
            Ingestion Worker                         │
                    │                                ▼
                    ▼                               RRF
             PDF parsing                            │
             Chunking                               ▼
             Embeddings                        No-op reranker
                    │                                │
                    ▼                                ▼
              PostgreSQL                       Context budget
                                                     │
                                                     ▼
                                              Prompt builder
                                                     │
                                                     ▼
                                                 Gemini
                                                     │
                                      ┌──────────────┴──────────────┐
                                      ▼                             ▼
                                  Generate                       SSE Stream
                                      │                             │
                                      └──────────────┬──────────────┘
                                                     ▼
                                                Citations
                                                     │
                                                     ▼
                                          messages + sources
```

For deeper architectural details, see [`docs/architecture.md`](docs/architecture.md).

---

## Core Features

### Document ingestion

* PDF-only document ingestion
* Tenant-scoped SHA-256 deduplication
* Versioned documents
* Backblaze B2 object storage
* Asynchronous processing through RabbitMQ
* Dead-letter queue for failed jobs
* Page-aware parent/child chunking
* Gemini embeddings
* Batched embedding requests with transient-error retries
* Transactional chunk persistence
* One active document version at a time

### Hybrid retrieval

retrievion combines two retrieval strategies:

```text
User question
     │
     ├───────────────┐
     ▼               ▼
Vector Search    Keyword Search
pgvector         PostgreSQL FTS
     │               │
     └───────┬───────┘
             ▼
          RRF Fusion
           K = 60
             │
             ▼
       Current no-op
          reranker
             │
             ▼
       Top retrieval
             │
             ▼
      Token-bounded
         context
```

Semantic search uses **pgvector cosine distance** with an HNSW index.

Keyword search uses PostgreSQL English full-text search with a GIN index.

The results are merged using **Reciprocal Rank Fusion (RRF)** with `K=60`.

> A reranker abstraction exists, but the current implementation uses a no-op reranker. A production cross-encoder/API reranker is not currently part of the default pipeline.

### RAG chat

A chat request follows this flow:

```text
Authenticate
    ↓
Validate conversation ownership
    ↓
Persist user message
    ↓
Embed question
    ↓
Hybrid retrieval
    ↓
Build bounded context
    ↓
Build grounded prompt
    ↓
Gemini generation
    ↓
Resolve [1], [2], ... citations
    ↓
Persist assistant message
    ↓
Persist message sources
    ↓
Return response
```

The same pipeline supports streaming responses through **Server-Sent Events (SSE)**.

Streaming events include:

* `message_start`
* `token`
* `citation`
* `message_complete`
* `error`

---

## Grounded Responses and Citations

Retrieved document content is explicitly treated as **untrusted context** by the prompt builder.

The model is instructed to:

* answer using the retrieved context,
* avoid following instructions contained inside retrieved documents,
* acknowledge when the available context is insufficient,
* reference supporting sources using `[1]`, `[2]`, etc.

After generation, the citation service maps those references back to retrieved chunks.

For example:

```text
Answer:
"The policy requires approval before deployment [1]."

        ↓

Citation resolver

        ↓

[1] → Retrieved chunk
        → Document
        → Page
        → Section
```

The resulting message/source relationships are persisted in PostgreSQL.

---

## Document Processing

Documents are stored separately from their extracted content.

```text
PDF
 │
 ├── Backblaze B2
 │
 └── PostgreSQL metadata
       │
       └── document version
              │
              ├── Parent chunks
              │
              └── Child chunks
                    │
                    └── Embeddings
```

Current chunking configuration:

| Setting             |      Value |
| ------------------- | ---------: |
| Parent unit         |   PDF page |
| Maximum child size  | 700 tokens |
| Minimum child size  |  50 tokens |
| Paragraph overlap   |        15% |
| Embedding dimension |       1536 |

Only child chunks receive embeddings in the current implementation.

---

## Storage

retrievion uses PostgreSQL as the primary system of record.

### PostgreSQL

Stores:

* tenants
* users and tenant memberships
* documents
* document versions
* document chunks
* embeddings
* conversations
* messages
* message sources
* ingestion jobs
* LLM telemetry
* evaluation datasets and results
* service clients

### pgvector

Embeddings are stored directly inside PostgreSQL:

```text
embedding vector(1536)
```

An HNSW index using cosine distance supports semantic retrieval.

### Backblaze B2

Original PDF files are stored in Backblaze B2 using its S3-compatible API.

Temporary download URLs can be generated through presigned requests.

### Redis

Redis is used for:

* opaque authentication tokens
* refresh/access token state
* OTPs
* counters and budget tracking
* other short-lived application state

### RabbitMQ

RabbitMQ provides asynchronous document ingestion.

The ingestion queue is durable and failures are routed to a dead-letter queue.

The current worker does **not** automatically requeue failed jobs.

---

## Multi-Tenancy

Users are global identities and can belong to multiple tenants.

```text
User
 │
 ▼
tenant_members
 │
 ▼
Tenant
 │
 ├── Documents
 ├── Conversations
 ├── Chunks
 ├── Ingestion Jobs
 └── LLM / Evaluation data
```

Tenant identifiers are carried through the authenticated request context and used to scope application data.

Conversation access is scoped to both:

```text
user_id + tenant_id
```

Retrieval is currently tenant-scoped.

---

## Authentication

The API uses an opaque-token architecture rather than exposing JWTs directly to clients.

```text
Client
  │
  │ Bearer opaque token
  ▼
Redis token lookup
  │
  ▼
Internal short-lived Ed25519 JWT
  │
  ▼
Application handlers
```

### User authentication

* bcrypt password hashing
* opaque access tokens
* opaque refresh tokens
* Redis-backed token storage
* token revocation
* 24-hour access-token lifetime
* 7-day refresh-token lifetime

### Internal authentication

After token introspection, the API exchanges the opaque token for a short-lived Ed25519-signed JWT used internally by the application.

Service clients can also authenticate using:

```text
client_id + client_secret
```

with bcrypt-protected secrets.

---

## LLM Integration

The current implementation integrates with Gemini through a dedicated HTTP client.

The LLM layer supports:

* normal generation
* structured generation
* streaming generation
* provider error classification
* transient retry handling
* configurable token/cost pricing
* token usage tracking
* latency tracking

For normal generation, transient provider/network failures are retried with exponential backoff and jitter.

Streaming requests are not retried after tokens have already started reaching the client.

---

## Evaluation

retrievion includes an evaluation pipeline for measuring retrieval quality and answer groundedness.

### Retrieval metrics

The evaluator calculates:

* Precision@K
* Recall@K
* Mean Reciprocal Rank (MRR)

### Faithfulness evaluation

Generated answers are evaluated by a separate LLM judge.

```text
Question
   │
   ▼
Retrieval
   │
   ├── Precision@K
   ├── Recall@K
   └── MRR
   │
   ▼
Generate answer
   │
   ▼
LLM faithfulness judge
   │
   ▼
0.0 ─────────────── 1.0
unsupported          grounded
```

The current faithfulness threshold is `0.7`.

The judge stores both:

* faithfulness score
* judge reasoning

This makes the evaluation result auditable rather than treating an LLM-generated score as unquestionable ground truth.

Evaluation runs and results are persisted in PostgreSQL.

Baseline comparison is also supported for detecting quality regressions.

---

## Observability

The project includes Prometheus metrics, OpenTelemetry tracing, and Sentry integration.

### Prometheus

Tracked signals include:

* LLM request count
* LLM errors
* input/output tokens
* estimated LLM cost
* LLM latency
* time to first token
* retrieval latency
* retrieval hit rate
* evaluation pass rate
* groundedness score

Example metrics:

```text
llm_requests_total
llm_tokens_input_total
llm_tokens_output_total
llm_latency_seconds
llm_ttft_seconds
retrieval_latency_seconds
evaluation_pass_rate
groundedness_score
```

### OpenTelemetry

The tracing package exports traces through OTLP/gRPC.

LLM spans can record:

* model
* provider
* input tokens
* output tokens
* latency
* finish reason

Retrieval spans can record:

* retrieved chunk IDs
* retrieval scores
* retrieval latency

### Sentry

The API integrates Sentry middleware for error reporting and request context.

---

## API and Worker

The application is split into two processes:

```text
┌───────────────────────┐
│         API           │
│                       │
│ HTTP / Gin            │
│ Authentication        │
│ Documents             │
│ Conversations         │
│ Chat                  │
└───────────┬───────────┘
            │
            │ RabbitMQ
            ▼
┌───────────────────────┐
│        Worker         │
│                       │
│ PDF processing        │
│ Chunking              │
│ Embeddings            │
│ Chunk persistence     │
└───────────────────────┘
```

The API runs database migrations during startup.

The worker consumes ingestion jobs and gracefully drains in-flight work during shutdown.

---

## Technology Stack

| Component        | Technology                        |
| ---------------- | --------------------------------- |
| Language         | Go                                |
| HTTP             | Gin                               |
| Database         | PostgreSQL 16                     |
| Vector search    | pgvector                          |
| Vector index     | HNSW                              |
| Keyword search   | PostgreSQL FTS + GIN              |
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

* Go
* Docker
* Docker Compose
* Gemini API key

### Clone

```bash
git clone https://github.com/MTariq99/retrievion.git
cd retrievion
```

### Configure environment

```bash
cp env.example .env
```

Set the required database, Redis, RabbitMQ, Gemini, and Backblaze B2 configuration.

At minimum, configure the application database and Gemini credentials before starting the services.

### Start infrastructure

```bash
docker compose -f deployments/docker-compose.yaml up -d postgres redis rabbitmq
```

### Run the API

```bash
go run ./cmd/api
```

The API listens on the configured HTTP port, which defaults to:

```text
8080
```

### Run the worker

In a second terminal:

```bash
go run ./cmd/worker
```

The worker consumes document-ingestion jobs from RabbitMQ.

---

## Docker Compose

The repository includes a complete local development environment:

```bash
docker compose -f deployments/docker-compose.yaml up --build
```

It provides:

* PostgreSQL + pgvector
* Redis
* RabbitMQ
* API
* ingestion worker

Persistent Docker volumes are configured for the infrastructure services.

---

## Project Structure

```text
retrievion/
├── cmd/
│   ├── api/
│   └── worker/
│
├── deployments/
│   └── docker-compose.yaml
│
├── docs/
│   └── architecture.md
│
├── internal/
│   ├── auth/
│   ├── chat/
│   ├── citation/
│   ├── conversation/
│   ├── document/
│   ├── embedding/
│   ├── evaluation/
│   ├── ingestion/
│   ├── llm/
│   ├── middleware/
│   ├── prompt/
│   ├── retrieval/
│   ├── storage/
│   └── tokenizer/
│
├── migrations/
├── models/
├── pkg/
├── tests/
├── Dockerfile
└── README.md
```

---

## Engineering Decisions

### Why PostgreSQL + pgvector?

The project keeps document metadata, chunks, embeddings, conversations, and citations in one transactional system.

This avoids introducing a separate vector database while still providing:

* vector similarity search
* HNSW indexing
* full-text search
* relational transactions
* JSONB metadata
* standard SQL access

### Why hybrid retrieval?

Semantic search handles conceptual similarity, while keyword search remains useful for:

* exact terminology
* names
* identifiers
* technical terms
* phrases that embeddings may underweight

RRF provides a simple way to combine both rankings.

### Why asynchronous ingestion?

PDF parsing and embedding generation are comparatively expensive operations.

Moving them behind RabbitMQ keeps document upload requests short and allows ingestion workers to process documents independently from the API.

### Why opaque tokens?

Clients never need to handle the application's internal JWT format.

The API can revoke and introspect opaque tokens through Redis, while the application still benefits from short-lived signed JWTs internally.

---

## Current Limitations

This project intentionally distinguishes implemented functionality from future production enhancements.

The current implementation does **not** include:

* a real cross-encoder/API reranker
* automatic ingestion retries
* query rewriting
* Kafka-based processing
* a separate vector database
* fully fine-grained per-document retrieval permissions
* automated CI/CD evaluation gating
* Kubernetes deployment manifests

These are potential extensions rather than features represented as currently implemented.

---

## Roadmap

Potential future improvements include:

* production reranking
* richer retrieval strategies
* query rewriting
* ingestion retry policies
* stronger document-level authorization
* automated evaluation gates
* retrieval/evaluation dashboards
* additional LLM and embedding providers
* Kubernetes deployment
* richer caching strategies
* improved evaluation datasets and judge calibration

---

## Documentation

Architecture and deeper implementation details:

```text
docs/
└── architecture.md
```

The README intentionally focuses on the implemented system and getting started. Detailed engineering rationale belongs in the architecture documentation.

---

## License

See the repository license for usage and distribution terms.
