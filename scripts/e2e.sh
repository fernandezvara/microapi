#!/usr/bin/env bash
# End-to-end test for Micro API
# - Verifies core CRUD, queries, schema validation, index management, and deletes
# - Acts as documentation: read and run step-by-step
#
# Requirements: curl, jq
# Usage: BASE=http://localhost:8080 SET=e2e COLL=people ./scripts/e2e.sh

set -euo pipefail

# --- Preconditions ---
for bin in curl jq; do
  if ! command -v "$bin" >/dev/null 2>&1; then
    echo "ERROR: required dependency not found: $bin" >&2
    exit 1
  fi
done

# --- Config ---
BASE=${BASE:-http://localhost:8080}
SET=${SET:-e2e}
COLL=${COLL:-people}
TIMEOUT_SEC=${TIMEOUT_SEC:-30}

# Globals to hold last request artifacts
_LAST_STATUS=""
_LAST_BODY_FILE=""
_LAST_HEADER_FILE=""

cleanup() {
  [[ -n "${_LAST_BODY_FILE}" && -f "${_LAST_BODY_FILE}" ]] && rm -f "${_LAST_BODY_FILE}" || true
  [[ -n "${_LAST_HEADER_FILE}" && -f "${_LAST_HEADER_FILE}" ]] && rm -f "${_LAST_HEADER_FILE}" || true
}
trap cleanup EXIT

log() { echo "[e2e] $*"; }

# Perform HTTP request
# args: METHOD URL [DATA]
http() {
  local method="$1"; shift
  local url="$1"; shift || true
  local data="${1:-}"; shift || true

  _LAST_BODY_FILE=$(mktemp)
  _LAST_HEADER_FILE=$(mktemp)

  if [[ -n "$data" ]]; then
    _LAST_STATUS=$(curl -sS -D "${_LAST_HEADER_FILE}" -o "${_LAST_BODY_FILE}" -w '%{http_code}' \
      -H 'Content-Type: application/json' -X "$method" --data "$data" "$url")
  else
    _LAST_STATUS=$(curl -sS -D "${_LAST_HEADER_FILE}" -o "${_LAST_BODY_FILE}" -w '%{http_code}' \
      -X "$method" "$url")
  fi
}

# GET with URL-encoded query parameters
# args: URL [key=val] ...
http_get_qs() {
  local url="$1"; shift || true
  _LAST_BODY_FILE=$(mktemp)
  _LAST_HEADER_FILE=$(mktemp)
  _LAST_STATUS=$(curl -sS -D "${_LAST_HEADER_FILE}" -o "${_LAST_BODY_FILE}" -w '%{http_code}' \
    --get "$url" "$@")
}

print_last() {
  echo "--- Status: ${_LAST_STATUS}"
  echo "--- Headers:"
  sed 's/^/  /' "${_LAST_HEADER_FILE}" | sed -n '1,20p'
  echo "--- Body:"
  if [[ -s "${_LAST_BODY_FILE}" ]]; then
    jq -C . <"${_LAST_BODY_FILE}" || cat "${_LAST_BODY_FILE}"
  else
    echo "<empty>"
  fi
}

assert_status() {
  local expect="$1"
  if [[ "${_LAST_STATUS}" != "$expect" ]]; then
    echo "ASSERT FAIL: expected HTTP $expect but got ${_LAST_STATUS}" >&2
    print_last
    exit 1
  fi
}

require_header_contains() {
  local name="$1"; shift
  local needle="$1"
  if ! grep -i "^$name:" "${_LAST_HEADER_FILE}" | grep -q "$needle"; then
    echo "ASSERT FAIL: header $name does not contain: $needle" >&2
    print_last
    exit 1
  fi
}

header_value() { # args: Header-Name
  local name="$1"
  awk -v IGNORECASE=1 -v n="$name" 'tolower($1)==tolower(n)":"{ $1=""; sub(/^ /,""); print }' "${_LAST_HEADER_FILE}" | tail -n1 | tr -d '\r'
}

extract_json() { # args: jq-filter
  local filter="$1"
  jq -r "$filter" <"${_LAST_BODY_FILE}"
}

# --- Begin E2E ---
log "Using BASE=$BASE SET=$SET COLL=$COLL"

log "1) Health"
http GET "$BASE/health"
assert_status 200
print_last

log "2) List sets"
http GET "$BASE/_sets"
assert_status 200
print_last

log "3) Create documents"
http POST "$BASE/$SET/$COLL" '{"name":"Alice","age":30,"user":{"id":"u1","email":"alice@example.com"}}'
assert_status 201
ALICE_ID=$(extract_json '.data._meta.id')
log "   Alice ID=$ALICE_ID"

http POST "$BASE/$SET/$COLL" '{"name":"Bob","age":25,"user":{"id":"u2","email":"bob@example.com"}}'
assert_status 201
BOB_ID=$(extract_json '.data._meta.id')
log "   Bob ID=$BOB_ID"

log "4) Query collection (limit, headers)"
http_get_qs "$BASE/$SET/$COLL" --data-urlencode "limit=10"
assert_status 200
XTOTAL=$(header_value 'X-Total-Items')
log "   X-Total-Items=$XTOTAL"

log "5) Filter: age >= 26"
WHERE='{"age":{"$gte":26}}'
http_get_qs "$BASE/$SET/$COLL" --data-urlencode "where=$WHERE"
assert_status 200
print_last

log "6) Debug plan header"
http_get_qs "$BASE/$SET/$COLL" --data-urlencode "debug=1" --data-urlencode "where=$WHERE"
assert_status 200
require_header_contains 'X-Query-Plan' ""

log "7) Get document by ID"
http GET "$BASE/$SET/$COLL/$ALICE_ID"
assert_status 200

log "8) Replace (PUT)"
http PUT "$BASE/$SET/$COLL/$ALICE_ID" '{"name":"Alice","age":31,"user":{"id":"u1","email":"alice@example.com"}}'
assert_status 200

log "9) Patch (PATCH)"
http PATCH "$BASE/$SET/$COLL/$ALICE_ID" '{"city":"Paris"}'
assert_status 200

log "10) Put JSON Schema (v6)"
read -r -d '' SCHEMA <<'JSON'
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": false,
  "required": ["name", "age", "user"],
  "properties": {
    "name": {"type": "string"},
    "age": {"type": "integer", "minimum": 0},
    "user": {
      "type": "object",
      "required": ["id", "email"],
      "properties": {
        "id": {"type": "string"},
        "email": {"type": "string", "format": "email"}
      },
      "additionalProperties": false
    },
    "city": {"type": "string"}
  }
}
JSON
http PUT "$BASE/$SET/$COLL/_schema" "$SCHEMA"
assert_status 200

log "11) Invalid update should fail (age < 0)"
http PATCH "$BASE/$SET/$COLL/$BOB_ID" '{"age":-5}'
assert_status 400

log "12) Valid update"
http PATCH "$BASE/$SET/$COLL/$BOB_ID" '{"age":26}'
assert_status 200

log "13) Create single-path index (user.email)"
http POST "$BASE/$SET/$COLL/_index" '{"path":"user.email"}'
assert_status 202

log "14) Create compound index (user.id,user.email)"
http POST "$BASE/$SET/$COLL/_index" '{"paths":["user.id","user.email"]}'
assert_status 202

log "15) Poll status for user.email index until ready/error"
start=$(date +%s)
while true; do
  http GET "$BASE/$SET/$COLL/_index/user.email"
  if [[ "${_LAST_STATUS}" == "200" ]]; then
    status=$(extract_json '.status')
    log "   status=$status"
    if [[ "$status" == "ready" ]]; then
      break
    elif [[ "$status" == "error" ]]; then
      echo "Index creation failed" >&2
      print_last
      exit 1
    fi
  elif [[ "${_LAST_STATUS}" == "404" ]]; then
    : # may not be inserted yet
  else
    echo "Unexpected status while polling: ${_LAST_STATUS}" >&2
    print_last
    exit 1
  fi
  now=$(date +%s)
  if (( now - start > TIMEOUT_SEC )); then
    echo "Timeout waiting for index readiness" >&2
    exit 1
  fi
  sleep 1
done

log "16) List indexes"
http GET "$BASE/$SET/$COLL/_indexes"
assert_status 200

log "17) Query using indexed path to increment usage"
WHERE_EMAIL='{"user.email":{"$eq":"alice@example.com"}}'
http_get_qs "$BASE/$SET/$COLL" --data-urlencode "where=$WHERE_EMAIL"
assert_status 200

log "18) Re-list indexes (usage_count should be >= 1 for matching index)"
http GET "$BASE/$SET/$COLL/_indexes"
assert_status 200
print_last

log "19) Delete single-path index"
http DELETE "$BASE/$SET/$COLL/_index/user.email"
assert_status 200

log "20) Delete compound index via paths"
# The path segment is ignored when ?paths=... is provided
http DELETE "$BASE/$SET/$COLL/_index/_?paths=user.id,user.email"
assert_status 200

log "21) Delete Bob by ID"
http DELETE "$BASE/$SET/$COLL/$BOB_ID"
assert_status 200

log "22) Conditional delete (age < 30)"
WHERE_DELETE='{"age":{"$lt":30}}'
http_get_qs "$BASE/$SET/$COLL" --data-urlencode "where=$WHERE_DELETE"
assert_status 200
COUNT=$(jq 'length' <"${_LAST_BODY_FILE}")
if [[ "$COUNT" -gt 0 ]]; then
  http DELETE "$BASE/$SET/$COLL?where=$(printf %s "$WHERE_DELETE" | jq -sRr @uri)"
  if [[ "${_LAST_STATUS}" == "200" ]]; then
    :
  elif [[ "${_LAST_STATUS}" == "403" ]]; then
    log "   Skipping conditional delete: collection deletion disabled (ALLOW_DELETE_COLLECTIONS=false)"
  else
    echo "Unexpected status for conditional delete: ${_LAST_STATUS}" >&2
    print_last
    exit 1
  fi
fi

log "23) Collection info"
http GET "$BASE/$SET/$COLL/_info"
assert_status 200
print_last

log "24) Done"
echo "SUCCESS: E2E completed"
