# MicroAPI

A lightweight, zero-ops JSON micro-database and API server. Store schemaless JSON documents in named sets and collections, query them with filters, optionally validate against JSON Schema, and add JSON-path indexes for speed. Ships with a built-in dashboard and an MCP interface.

- **Storage**: SQLite with WAL, one physical table per set: `data_<set>` storing `{id, collection, data JSON, created_at, updated_at}`.
- **API**: Clean REST endpoints for documents, collections, sets, indexing, and schemas.
- **Query**: JSON where filters with rich operators ($eq, $ne, $gt, $gte, $lt, $lte, $like, $ilike, $startsWith, $istartsWith, $endsWith, $iendsWith, $contains, $icontains, $in, $nin, $between, $isNull, $notNull), plus order, limit/offset, and pagination header.
- **Indexes**: Async JSON-path indexes tracked in metadata and usage-counted for observability.
- **Validation**: Optional per-collection JSON Schema validation on create/update/replace.
- **Dashboard**: Single-page UI served from `/` for exploring data and testing APIs.
- **MCP**: Two flavors: HTTP endpoints at `/mcp` and a standalone stdio MCP server (`cmd/micro-api-mcp`).
- **Lua Functions**: Execute custom business logic with Lua scripts that have full database access, transaction support, and sandboxed execution.

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

- Comparisons: `$eq`, `$ne`, `$gt`, `$gte`, `$lt`, `$lte`
- Text pattern:
  - `$like` (SQL LIKE pattern e.g. "%foo%")
  - `$ilike` (case-insensitive LIKE)
  - `$startsWith`, `$istartsWith`
  - `$endsWith`, `$iendsWith`
  - `$contains`, `$icontains` (wraps value with `%...%`)
- Sets: `$in`, `$nin` (expects an array of values)
- Range: `$between` (expects `[min, max]`)
- Null checks: `$isNull`, `$notNull` (value ignored)

Notes:

- `$in` with an empty array matches no rows; `$nin` with an empty array matches all rows.
- Case-insensitive operators use `LOWER(...)` under the hood.
- In `where`, you can use dot paths like `user.age` or JSONPath (e.g. `$.user.age`).
- For `order_by` and index endpoints, JSONPath is accepted (e.g. `$.user.age`).

Paths:

- `where` keys can use dot notation (`user.age`) or JSONPath (`$.user.age`).
- `order_by` and index endpoints accept JSONPath (e.g. `$.user.age`).

#### Errors and edge-cases

- Malformed `where` returns HTTP 400 with a friendly message:
  - `"malformed where clause: expected a JSON object where keys are field paths and values are operator objects"`
- Unsupported operator returns HTTP 400, e.g. `"unsupported operator: $foo"`.
- Empty results always return an empty array `[]` (never `null`).
- Multi-argument expectations:
  - `$in`/`$nin` require an array value.
  - `$between` requires a two-element array `[min, max]`.

Examples of bad requests:

```bash
# Malformed JSON
curl "http://localhost:8080/myset/users?where=not-json"

# Unsupported operator
curl "http://localhost:8080/myset/users?where={\"age\":{\"$foo\":1}}"

# Wrong shape for $between (should be [min,max])
curl "http://localhost:8080/myset/users?where={\"age\":{\"$between\":42}}"
```

Examples:

```bash
# All docs where user.age >= 18, newest first, first page of 10
curl "http://localhost:8080/myset/users?where={\"user.age\":{\"$gte\":18}}&order_by=created_at&limit=10&offset=0"

# Suppress metadata
curl "http://localhost:8080/myset/users?where={}&meta=0"

# Text pattern matching
curl "http://localhost:8080/myset/users?where={\"user.name\":{\"$contains\":\"Ada\"}}"
curl "http://localhost:8080/myset/users?where={\"user.name\":{\"$icontains\":\"ada\"}}"   # case-insensitive
curl "http://localhost:8080/myset/users?where={\"user.name\":{\"$startsWith\":\"An\"}}"
curl "http://localhost:8080/myset/users?where={\"user.name\":{\"$istartsWith\":\"an\"}}"  # case-insensitive
curl "http://localhost:8080/myset/users?where={\"user.email\":{\"$endsWith\":\"@example.com\"}}"
curl "http://localhost:8080/myset/users?where={\"user.email\":{\"$iendsWith\":\"@EXAMPLE.COM\"}}"  # case-insensitive

# Set membership
curl "http://localhost:8080/myset/orders?where={\"status\":{\"$in\":[\"new\",\"processing\"]}}"
curl "http://localhost:8080/myset/orders?where={\"status\":{\"$nin\":[\"cancelled\"]}}"

# Range
curl "http://localhost:8080/myset/products?where={\"price\":{\"$between\":[10,20]}}"

# Null checks
curl "http://localhost:8080/myset/users?where={\"archivedAt\":{\"$isNull\":true}}"
curl "http://localhost:8080/myset/users?where={\"archivedAt\":{\"$notNull\":true}}"

# Order by JSON path
curl "http://localhost:8080/myset/users?order_by=$.user.age"

# Pagination and debug header
curl -i "http://localhost:8080/myset/users?limit=5&offset=5&debug=1"
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

## Lua Functions

Create and execute custom business logic functions using Lua. Functions are scoped to a set, can perform complex database operations, and execute within transactions.

### Function Endpoints

- **POST** `/{set}/_functions` → create a function
- **GET** `/{set}/_functions` → list all functions in set
- **GET** `/{set}/_functions/{id}` → get function definition
- **PUT** `/{set}/_functions/{id}` → update function
- **DELETE** `/{set}/_functions/{id}` → delete function
- **POST** `/{set}/_functions/{id}` → execute function
- **POST** `/{set}/_functions/_sandbox` → test function without persisting changes
- **POST** `/{set}/_functions/_import` → import functions
- **GET** `/{set}/_functions?export=true` → export all functions
- **GET** `/{set}/_functions/{id}?export=true` → export single function

### Function Definition

```json
{
  "id": "function_name",
  "name": "Display Name",
  "description": "What it does",
  "code": "lua code here",
  "timeout": 5000,
  "input_schema": { }
}
```

- **id**: Unique identifier (alphanumeric + underscore only)
- **code**: Lua script to execute (required)
- **timeout**: Max execution time in milliseconds (default: 5000, max: 30000)
- **input_schema**: Optional JSON Schema for input validation

### Lua API

Functions have access to the following operations within their set:

**Database Operations:**
```lua
-- Query documents
local results = microapi.query("collection", {field = "value"})

-- Get document by ID
local doc = microapi.get("collection", "doc_id")

-- Create document
local created = microapi.create("collection", {name = "Alice", age = 30})

-- Update document (full replace)
local updated = microapi.update("collection", "doc_id", {name = "Alice", age = 31})

-- Patch document (merge)
local patched = microapi.patch("collection", "doc_id", {age = 31})

-- Delete document
local success = microapi.delete("collection", "doc_id")
```

**Utilities:**
```lua
-- JSON encoding/decoding
local json_str = json.encode({key = "value"})
local data = json.decode('{"key":"value"}')

-- Logging
log.info("Information message")
log.error("Error message")
```

**Global Variables:**
```lua
-- Input data (from request body)
local value = input.field_name

-- Current set name (read-only)
local current_set = set

-- Execution context (read-only)
local func_id = ctx.function_id
local exec_id = ctx.execution_id
local timestamp = ctx.timestamp

-- Set HTTP response code
http_status = 200  -- or 201, 400, 404, 409, 500, etc.

-- Set response data
output = {
  message = "Success",
  data = result
}
```

### Transaction Behavior

All database operations within a function execute in a SQL transaction:

- **Commits** when `http_status` is 2xx (200-299) and no errors
- **Rolls back** when `http_status` is 4xx/5xx (400+)
- **Rolls back** on Lua runtime errors
- **Rolls back** on timeout
- **Always rolls back** in sandbox mode

This ensures atomic operations and data consistency.

### Example: Shopping Cart Function

```bash
# Create function
curl -X POST http://localhost:8080/shop/_functions \
  -H 'Content-Type: application/json' \
  -d '{
    "id": "add_to_cart",
    "name": "Add to Cart",
    "code": "local product = microapi.get(\"products\", input.product_id)\nif not product then\n  http_status = 404\n  output = {error = \"Product not found\"}\n  return\nend\n\nlocal carts = microapi.query(\"carts\", {user_id = input.user_id})\nlocal cart\nif #carts == 0 then\n  cart = microapi.create(\"carts\", {user_id = input.user_id, items = {}, total = 0})\nelse\n  cart = carts[1]\nend\n\ntable.insert(cart.items, {product_id = input.product_id, quantity = input.quantity})\ncart = microapi.update(\"carts\", cart._meta.id, cart)\n\nhttp_status = 200\noutput = {cart = cart}",
    "timeout": 5000
  }'

# Execute function
curl -X POST http://localhost:8080/shop/_functions/add_to_cart \
  -H 'Content-Type: application/json' \
  -d '{
    "user_id": "user_123",
    "product_id": "prod_456",
    "quantity": 2
  }'

# Test in sandbox (no changes persisted)
curl -X POST http://localhost:8080/shop/_functions/_sandbox \
  -H 'Content-Type: application/json' \
  -d '{
    "code": "http_status = 200\noutput = {test = true}",
    "input": {}
  }'

# Export functions
curl http://localhost:8080/shop/_functions?export=true

# Import functions
curl -X POST http://localhost:8080/shop/_functions/_import \
  -H 'Content-Type: application/json' \
  -d '{
    "version": "1.0",
    "functions": [...],
    "options": {"overwrite": false, "validate": true}
  }'
```

### Security & Isolation

- **Sandboxed**: No file system, network, or OS access
- **Set-scoped**: Functions can only access data within their set
- **Timeout enforced**: Maximum execution time of 30 seconds
- **Code validation**: Syntax checking before save
- **Pattern blocking**: Dangerous patterns (require, dofile, etc.) are blocked

### Monitoring

Each function tracks execution statistics:
- Total executions
- Success/error counts
- Success rate
- Average duration
- Error breakdown by HTTP status code

View stats by getting the function definition:
```bash
curl http://localhost:8080/shop/_functions/add_to_cart | jq '.data.stats'
```

### Additional Resources

- Full documentation: `lua_functions_integration.md`
- Examples: `examples/lua-functions/`
- E2E tests: `scripts/e2e_functions.sh`
- Quick reference: `examples/lua-functions/QUICK_REFERENCE.md`

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
- `internal/luafn/`: Lua functions service, storage, and handlers.
- `internal/query/`: where parser and SQL builder.
- `internal/database/`: connection, migrations, indexes, per-set table helpers.
- `internal/validation/`: JSON Schema persistence and validation.
- `web/static/`: dashboard (`dashboard.html`, `style.css`).
- `examples/lua-functions/`: Lua functions examples and documentation.
- `scripts/`: E2E test scripts for core API and functions.
- `docker-compose.yaml`: local stack with optional n8n.

## n8n integration

n8n integration is provided via the `n8n-nodes-microapi` community node. repo: https://github.com/fernandezvara/n8n-nodes-microapi

## License

MIT. See `LICENSE`.
