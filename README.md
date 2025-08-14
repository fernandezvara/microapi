# MicroAPI

A lightweight, zero-ops JSON micro-database and API server. Store schemaless JSON documents in named sets and collections, query them with filters, optionally validate against JSON Schema, and add JSON-path indexes for speed. Ships with a built-in dashboard and an MCP interface.

- **Storage**: SQLite with WAL, one physical table per set: `data_<set>` storing `{id, collection, data JSON, created_at, updated_at}`.
- **API**: Clean REST endpoints for documents, collections, sets, indexing, and schemas.
- **Query**: JSON where filters with operators ($eq, $ne, $gt, $gte, $lt, $lte), order, limit/offset, pagination header.
- **Indexes**: Async JSON-path indexes tracked in metadata and usage-counted for observability.
- **Validation**: Optional per-collection JSON Schema validation on create/update/replace.
- **Dashboard**: Single-page UI served from `/` for exploring data and testing APIs.
- **MCP**: Two flavors: HTTP endpoints at `/mcp` and a standalone stdio MCP server (`cmd/micro-api-mcp`).

## Quick start

### Run with Docker

```bash
# Persist DB to ./data.db and expose on 8080
docker run --rm -p 8080:8080 \
  -v "$(pwd)":/data \
  -e DB_PATH=/data/data.db \
  ghcr.io/fernandezvara/microapi:latest
```

- Health: `curl http://localhost:8080/health`
- Dashboard: open http://localhost:8080/

### Docker Compose (includes optional n8n)

See `docker-compose.yaml`. Start both services:

```bash
docker compose up --build
```

The Compose file mounts a community n8n node from `../n8n-nodes-microapi` for convenience.

### Local dev

```bash
make run         # builds and runs ./cmd/micro-api
make test        # run tests
make css         # builds web/static/style.css via Tailwind (requires Node + npx)
```

Go 1.24+ recommended. Config is read from environment and optional `.env`.

## Configuration

Defined in `internal/config/config.go`.

- **PORT** (default `8080`): HTTP port.
- **DB_PATH** (default `./data.db`): SQLite file path.
- **MAX_REQUEST_SIZE** (default `1048576`): Max request body bytes.
- **ALLOW_DELETE_SETS** (default `false`): Enable `DELETE /{set}`.
- **ALLOW_DELETE_COLLECTIONS** (default `false`): Enable `DELETE /{set}/{collection}`.
- **CORS** (default empty): CSV of allowed Origins. Empty means reflect any Origin.
- **DEV** (default `false`): Dev mode flag (currently used for minor toggles).

CORS exposes the `X-Total-Items` header to browsers.

## Data model

- **Set**: Top-level namespace. Backed by table `data_<set>`.
- **Collection**: Logical group inside a set. Stored in the `collection` column.
- **Document**: Arbitrary JSON in `data` column plus generated metadata timestamps.

SQLite is opened with WAL mode, foreign keys ON, busy timeout, and synchronous NORMAL (`internal/database/connection.go`). Per-set tables have helpful indexes on `collection` and `(collection, created_at)`.

## Response envelope

Every response uses `internal/models.APIResponse`:

```json
{ "success": true, "data": ..., "error": null }
```

- Errors return `success:false` with `error` message and appropriate HTTP status.
- Document responses include `_meta` unless suppressed (see below).

## REST API

Base path is `/`. Names (`{set}`, `{collection}`) must match `^[a-zA-Z0-9_]+$`.

### Health

- GET `/health` → `{ status: "ok", version: <string> }`

### Dashboard

- GET `/` serves the UI. Assets: `/style.css`, `/favicon.ico`, `/logo.svg`.

### Sets

- GET `/_sets` → summary of sets with collection/doc counts.
- GET `/{set}` → per-collection stats for the set.
- DELETE `/{set}` → drops the set table and metadata. Requires `ALLOW_DELETE_SETS=true`.

Example:

```bash
curl http://localhost:8080/_sets
curl http://localhost:8080/myset
```

### Collections & Documents

- POST `/{set}/{collection}` → create document.
- GET `/{set}/{collection}` → query documents (see Query section).
- GET `/{set}/{collection}/{id}` → fetch one.
- PUT `/{set}/{collection}/{id}` → replace document (full body).
- PATCH `/{set}/{collection}/{id}` → merge patch.
- DELETE `/{set}/{collection}/{id}` → delete by id.
- DELETE `/{set}/{collection}` → delete all or filtered (requires `ALLOW_DELETE_COLLECTIONS=true`).

Metadata in responses:

- `_meta.id`, `_meta.created_at`, `_meta.updated_at` are added by default.
- Suppress with `?meta=0` on GET/Query endpoints.

### Querying

Endpoint: `GET /{set}/{collection}` with query params:

- `where`: JSON string. Example: `{"user.name": {"$eq": "Alice"}}`
- `order_by`: `created_at`, `updated_at`, or a JSON path like `$.user.age`
- `limit`: integer > 0
- `offset`: integer ≥ 0
- `debug=1`: adds `X-Query-Plan` header with `EXPLAIN QUERY PLAN` summary

Pagination:

- Response header `X-Total-Items` includes the total count ignoring limit/offset.

Supported operators in `where`:

- `$eq`, `$ne`, `$gt`, `$gte`, `$lt`, `$lte`

Paths:

- You can use dot notation (`user.age`) or JSONPath (`$.user.age`).

Examples:

```bash
# All docs where user.age >= 18, newest first, first page of 10
curl "http://localhost:8080/myset/users?where={\"user.age\":{\"$gte\":18}}&order_by=created_at&limit=10&offset=0"

# Suppress metadata
curl "http://localhost:8080/myset/users?where={}&meta=0"
```

### Index management

Create, inspect, and remove JSON-path indexes per collection. Index creation is asynchronous.

- POST `/{set}/{collection}/_index`
  - Body: `{ "path": "$.user.age" }` or `{ "paths": ["$.user.age", "$.country"] }`
  - Response: `202 Accepted`, `{ name, status: "creating" }`
- GET `/{set}/{collection}/_indexes` → list index metadata.
- GET `/{set}/{collection}/_index/{path}` → status for an index.
  - `path` is URL-encoded JSONPath. Example for `$.user.age`: `%24.user.age`
- DELETE `/{set}/{collection}/_index/{path}` or `DELETE .../_index/{path}?paths=p1,p2` → drops index and metadata.

Index metadata fields (`internal/database/index.go`):

- `paths` (CSV), `status` (`creating|ready|error`), `error`, `usage_count`, `last_used_at`, `created_at`.

### Schema management

- PUT `/{set}/{collection}/_schema`
  - Body: JSON Schema to enable validation, or `null`/empty to remove schema.
  - Response: `{ schema: <echoed-or-null> }`
- GET `/{set}/{collection}/_info`
  - Response: `{ schema, indexes, stats: { count, created_at? } }`

Documents are validated on create/replace/update when a schema is set.

## MCP

Planned for future versions, unstable.

**Stdio Server** (`cmd/micro-api-mcp`)

- Build: `make build` (produces `bin/micro-api-mcp`)
- Run under an MCP-capable client (stdio transport). Provides the same tools against the configured SQLite DB.

## Validation, limits, and CORS

- **Name validation**: set/collection must match `^[a-zA-Z0-9_]+$`.
- **Reserved fields**: top-level keys starting with `_` are reserved in documents. `_meta` in request bodies is ignored/validated and never stored.
- **Body limit**: `MAX_REQUEST_SIZE` enforced via middleware.
- **CORS**: Allow list via `CORS` env var; `X-Total-Items` is exposed for pagination.

## Build & release

- Multi-stage `Dockerfile` builds a static binary and exposes port 8080.
- Published images: `ghcr.io/fernandezvara/microapi:<tag>` with multi-arch manifest (`latest` tag points to the newest release).
- `Makefile` targets: `build`, `run`, `test`, `css`.

## Examples

```bash
# Create a document
curl -X POST http://localhost:8080/myset/users \
  -H 'Content-Type: application/json' \
  -d '{"user":{"name":"Ada","age":37}}'

# Get it back (replace <id>)
curl http://localhost:8080/myset/users/<id>

# Query (age >= 30), order by JSON path
curl "http://localhost:8080/myset/users?where={\"user.age\":{\"$gte\":30}}&order_by=$.user.age"

# Create an index
curl -X POST http://localhost:8080/myset/users/_index \
  -H 'Content-Type: application/json' \
  -d '{"path":"$.user.age"}'

# Set a JSON Schema
curl -X PUT http://localhost:8080/myset/users/_schema \
  -H 'Content-Type: application/json' \
  -d '{"type":"object","properties":{"user":{"type":"object","properties":{"name":{"type":"string"},"age":{"type":"number"}}}},"required":["user"]}'
```

## Project structure

- `cmd/micro-api/`: HTTP server main.
- `cmd/micro-api-mcp/`: MCP stdio server main.
- `internal/server/server.go`: router and middleware wiring.
- `internal/handlers/`: REST and MCP handlers.
- `internal/query/`: where parser and SQL builder.
- `internal/database/`: connection, migrations, indexes, per-set table helpers.
- `internal/validation/`: JSON Schema persistence and validation.
- `web/static/`: dashboard (`dashboard.html`, `style.css`).
- `docker-compose.yaml`: local stack with optional n8n.

## n8n integration

n8n integration is provided via the `n8n-nodes-microapi` community node. repo: https://github.com/fernandezvara/n8n-nodes-microapi

## License

MIT. See `LICENSE`.
