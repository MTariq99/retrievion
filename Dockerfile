# ============================================================
# Build stage
# ============================================================
FROM golang:1.26-alpine AS builder

WORKDIR /src

RUN apk add --no-cache ca-certificates

# Dependency layer
COPY go.mod go.sum ./
RUN go mod download

# Application source
COPY . .

# Build API
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/api \
    ./cmd/api

# Build worker
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/worker \
    ./cmd/worker


# ============================================================
# API runtime
# ============================================================
FROM alpine:3.22 AS api

RUN apk add --no-cache ca-certificates \
    && adduser -D -H -u 10001 app

WORKDIR /app

COPY --from=builder /out/api /app/api
COPY --from=builder /src/migrations /app/migrations

USER app

EXPOSE 8080

ENTRYPOINT ["/app/api"]


# ============================================================
# Worker runtime
# ============================================================
FROM alpine:3.22 AS worker

RUN apk add --no-cache ca-certificates \
    && adduser -D -H -u 10001 app

WORKDIR /app

COPY --from=builder /out/worker /app/worker

USER app

ENTRYPOINT ["/app/worker"]