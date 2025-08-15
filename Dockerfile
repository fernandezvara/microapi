# syntax=docker/dockerfile:1

# --- Build stage ---
FROM docker.io/library/golang:1.24.6-alpine AS builder
WORKDIR /app

ENV CGO_ENABLED=0
ENV GO111MODULE=on

# Cache deps first
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

# Copy the rest of the source
COPY cmd/ ./cmd/
COPY web/ ./web/
COPY internal/ ./internal/

# Build the micro API binary
RUN --mount=type=cache,target=/go/pkg/mod go build -ldflags="-s -w" -o micro-api ./cmd/micro-api

# --- Runtime stage ---
FROM scratch

ENV PORT=8080
ENV DB_PATH=/data/data.db

# Working directory is root; DB will be at /data/data.db (mounted volume)
WORKDIR /

# Copy binary from builder
COPY --from=builder /app/micro-api /micro-api

EXPOSE 8080
VOLUME ["/data"]

ENTRYPOINT ["/micro-api"]
