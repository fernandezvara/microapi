#!/usr/bin/env bash
# End-to-end test for Micro API Lua Functions
# - Verifies function CRUD, execution, transactions, sandbox mode, export/import
# - Acts as documentation: read and run step-by-step
#
# Requirements: curl, jq
# Usage: BASE=http://localhost:8080 SET=e2efn ./scripts/e2e_functions.sh

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
SET=${SET:-e2efn}
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

log() { echo "[e2e-functions] $*"; }

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

extract_json() { # args: jq-filter
  local filter="$1"
  jq -r "$filter" <"${_LAST_BODY_FILE}"
}

assert_json_equals() { # args: jq-filter expected-value
  local filter="$1"
  local expected="$2"
  local actual
  actual=$(extract_json "$filter")
  if [[ "$actual" != "$expected" ]]; then
    echo "ASSERT FAIL: jq '$filter' expected '$expected' but got '$actual'" >&2
    print_last
    exit 1
  fi
}

assert_json_contains() { # args: jq-filter substring
  local filter="$1"
  local substring="$2"
  local actual
  actual=$(extract_json "$filter")
  if [[ "$actual" != *"$substring"* ]]; then
    echo "ASSERT FAIL: jq '$filter' does not contain '$substring', got: '$actual'" >&2
    print_last
    exit 1
  fi
}

# --- Begin E2E ---
log "Using BASE=$BASE SET=$SET"
log ""
log "========================================="
log "  Lua Functions E2E Test Suite"
log "========================================="
log ""

# --- Test 1: Create a simple function ---
log "TEST 1: Create a simple echo function"
http POST "$BASE/$SET/_functions" '{
  "id": "echo_func",
  "name": "Echo Function",
  "description": "Returns the input with a greeting",
  "code": "http_status = 200\noutput = {\n  greeting = \"Hello, \" .. (input.name or \"World\"),\n  timestamp = ctx.timestamp\n}",
  "timeout": 5000
}'
assert_status 201
assert_json_equals '.success' 'true'
assert_json_equals '.data.id' 'echo_func'
log "✓ Function created successfully"
print_last
log ""

# --- Test 2: List functions ---
log "TEST 2: List all functions"
http GET "$BASE/$SET/_functions"
assert_status 200
assert_json_equals '.success' 'true'
FUNC_COUNT=$(extract_json '.data | length')
log "✓ Found $FUNC_COUNT function(s)"
print_last
log ""

# --- Test 3: Get specific function ---
log "TEST 3: Get function by ID"
http GET "$BASE/$SET/_functions/echo_func"
assert_status 200
assert_json_equals '.success' 'true'
assert_json_equals '.data.id' 'echo_func'
assert_json_equals '.data.name' 'Echo Function'
log "✓ Function retrieved successfully"
log ""

# --- Test 4: Execute the function ---
log "TEST 4: Execute echo function"
http POST "$BASE/$SET/_functions/echo_func" '{
  "name": "Alice"
}'
assert_status 200
assert_json_equals '.success' 'true'
assert_json_contains '.data.greeting' 'Hello, Alice'
log "✓ Function executed successfully"
print_last
log ""

# --- Test 5: Create function with database operations ---
log "TEST 5: Create function with database operations"
http POST "$BASE/$SET/_functions" '{
  "id": "create_product",
  "name": "Create Product",
  "description": "Creates a product and returns it",
  "code": "if not input.name or not input.price then\n  http_status = 400\n  output = {error = \"Missing required fields\"}\n  return\nend\n\nlocal product = microapi.create(\"products\", {\n  name = input.name,\n  price = input.price,\n  stock = input.stock or 0\n})\n\nhttp_status = 201\noutput = {\n  product = product,\n  message = \"Product created\"\n}",
  "timeout": 5000
}'
assert_status 201
assert_json_equals '.success' 'true'
log "✓ Function with DB operations created"
log ""

# --- Test 6: Execute function with database operations ---
log "TEST 6: Execute function to create product"
http POST "$BASE/$SET/_functions/create_product" '{
  "name": "Widget",
  "price": 29.99,
  "stock": 100
}'
assert_status 201
assert_json_equals '.success' 'true'
PRODUCT_ID=$(extract_json '.data.product._meta.id')
log "✓ Product created with ID: $PRODUCT_ID"
print_last
log ""

# --- Test 7: Verify product was created ---
log "TEST 7: Verify product exists in database"
http GET "$BASE/$SET/products/$PRODUCT_ID"
assert_status 200
assert_json_equals '.success' 'true'
assert_json_equals '.data.name' 'Widget'
log "✓ Product verified in database"
log ""

# --- Test 8: Create function that queries database ---
log "TEST 8: Create function that queries products"
http POST "$BASE/$SET/_functions" '{
  "id": "list_products",
  "name": "List Products",
  "description": "Lists all products",
  "code": "local products = microapi.query(\"products\", {})\n\nhttp_status = 200\noutput = {\n  products = products,\n  count = #products\n}",
  "timeout": 5000
}'
assert_status 201
log "✓ Query function created"
log ""

# --- Test 9: Execute query function ---
log "TEST 9: Execute query function"
http POST "$BASE/$SET/_functions/list_products" '{}'
assert_status 200
assert_json_equals '.success' 'true'
PRODUCT_COUNT=$(extract_json '.data.count')
log "✓ Found $PRODUCT_COUNT product(s)"
print_last
log ""

# --- Test 10: Create function that will fail (rollback test) ---
log "TEST 10: Create function that tests transaction rollback"
http POST "$BASE/$SET/_functions" '{
  "id": "test_rollback",
  "name": "Test Rollback",
  "description": "Creates data then returns error to test rollback",
  "code": "microapi.create(\"test_rollback_data\", {\n  data = \"This should be rolled back\"\n})\n\nhttp_status = 400\noutput = {error = \"Intentional error for rollback test\"}",
  "timeout": 5000
}'
assert_status 201
log "✓ Rollback test function created"
log ""

# --- Test 11: Execute rollback function ---
log "TEST 11: Execute rollback function (should fail)"
http POST "$BASE/$SET/_functions/test_rollback" '{}'
assert_status 400
assert_json_equals '.success' 'false'
log "✓ Function returned error as expected"
log ""

# --- Test 12: Verify rollback occurred ---
log "TEST 12: Verify data was rolled back"
http GET "$BASE/$SET/test_rollback_data"
assert_status 200
ROLLBACK_COUNT=$(extract_json '.data | length')
if [[ "$ROLLBACK_COUNT" != "0" ]]; then
  echo "ASSERT FAIL: Expected 0 documents (rollback), but found $ROLLBACK_COUNT" >&2
  exit 1
fi
log "✓ Transaction rollback confirmed (no data persisted)"
log ""

# --- Test 13: Test sandbox mode ---
log "TEST 13: Test sandbox mode (no data persisted)"
http POST "$BASE/$SET/_functions/_sandbox" '{
  "code": "local product = microapi.create(\"sandbox_products\", {\n  name = \"Sandbox Product\",\n  price = 99.99\n})\n\nhttp_status = 200\noutput = {\n  product_id = product._meta.id,\n  message = \"Created in sandbox\"\n}",
  "input": {},
  "timeout": 5000
}'
assert_status 200
assert_json_equals '.success' 'true'
assert_json_contains '.data.warning' 'Sandbox mode'
log "✓ Sandbox execution successful"
print_last
log ""

# --- Test 14: Verify sandbox data was not persisted ---
log "TEST 14: Verify sandbox data was not persisted"
http GET "$BASE/$SET/sandbox_products"
assert_status 200
SANDBOX_COUNT=$(extract_json '.data | length')
if [[ "$SANDBOX_COUNT" != "0" ]]; then
  echo "ASSERT FAIL: Expected 0 sandbox documents, but found $SANDBOX_COUNT" >&2
  exit 1
fi
log "✓ Sandbox rollback confirmed (no data persisted)"
log ""

# --- Test 15: Update function ---
log "TEST 15: Update existing function"
http PUT "$BASE/$SET/_functions/echo_func" '{
  "id": "echo_func",
  "name": "Echo Function v2",
  "description": "Updated echo function",
  "code": "http_status = 200\noutput = {\n  greeting = \"Greetings, \" .. (input.name or \"Stranger\"),\n  version = 2\n}",
  "timeout": 3000
}'
assert_status 200
assert_json_equals '.success' 'true'
assert_json_equals '.data.name' 'Echo Function v2'
log "✓ Function updated successfully"
log ""

# --- Test 16: Execute updated function ---
log "TEST 16: Execute updated function"
http POST "$BASE/$SET/_functions/echo_func" '{
  "name": "Bob"
}'
assert_status 200
assert_json_contains '.data.greeting' 'Greetings, Bob'
assert_json_equals '.data.version' '2'
log "✓ Updated function works correctly"
log ""

# --- Test 17: Test function with error handling ---
log "TEST 17: Create function with input validation"
http POST "$BASE/$SET/_functions" '{
  "id": "validate_input",
  "name": "Validate Input",
  "code": "if not input.email then\n  http_status = 400\n  output = {error = \"Email is required\"}\n  return\nend\n\nif not string.find(input.email, \"@\") then\n  http_status = 422\n  output = {error = \"Invalid email format\"}\n  return\nend\n\nhttp_status = 200\noutput = {message = \"Email is valid\", email = input.email}",
  "timeout": 5000
}'
assert_status 201
log "✓ Validation function created"
log ""

# --- Test 18: Test validation - missing field ---
log "TEST 18: Test validation with missing field"
http POST "$BASE/$SET/_functions/validate_input" '{}'
assert_status 400
assert_json_equals '.success' 'false'
assert_json_contains '.error' 'Email is required'
log "✓ Validation correctly rejected missing field"
log ""

# --- Test 19: Test validation - invalid format ---
log "TEST 19: Test validation with invalid format"
http POST "$BASE/$SET/_functions/validate_input" '{
  "email": "invalid-email"
}'
assert_status 422
assert_json_contains '.error' 'Invalid email format'
log "✓ Validation correctly rejected invalid format"
log ""

# --- Test 20: Test validation - valid input ---
log "TEST 20: Test validation with valid input"
http POST "$BASE/$SET/_functions/validate_input" '{
  "email": "test@example.com"
}'
assert_status 200
assert_json_equals '.success' 'true'
log "✓ Validation accepted valid input"
log ""

# --- Test 21: Export single function ---
log "TEST 21: Export single function"
http_get_qs "$BASE/$SET/_functions/echo_func" --data-urlencode "export=true"
assert_status 200
assert_json_equals '.success' 'true'
assert_json_equals '.data.version' '1.0'
assert_json_equals '.data.function.id' 'echo_func'
log "✓ Function exported successfully"
print_last
log ""

# --- Test 22: Export all functions ---
log "TEST 22: Export all functions"
http_get_qs "$BASE/$SET/_functions" --data-urlencode "export=true"
assert_status 200
assert_json_equals '.success' 'true'
assert_json_equals '.data.version' '1.0'
assert_json_equals '.data.set' "$SET"
EXPORT_COUNT=$(extract_json '.data.functions | length')
log "✓ Exported $EXPORT_COUNT function(s)"
log ""

# --- Test 23: Save export for import test ---
log "TEST 23: Save export to file"
EXPORT_FILE=$(mktemp)
extract_json '.data' > "$EXPORT_FILE"
log "✓ Export saved to $EXPORT_FILE"
log ""

# --- Test 24: Delete a function ---
log "TEST 24: Delete function"
http DELETE "$BASE/$SET/_functions/validate_input"
assert_status 200
assert_json_equals '.success' 'true'
log "✓ Function deleted successfully"
log ""

# --- Test 25: Verify deletion ---
log "TEST 25: Verify function was deleted"
http GET "$BASE/$SET/_functions/validate_input"
assert_status 404
log "✓ Function not found (correctly deleted)"
log ""

# --- Test 26: Import functions ---
log "TEST 26: Import functions from export"
IMPORT_DATA=$(cat "$EXPORT_FILE" | jq '{version: .version, functions: .functions, options: {overwrite: true, validate: true}}')
http POST "$BASE/$SET/_functions/_import" "$IMPORT_DATA"
assert_status 200
assert_json_equals '.success' 'true'
IMPORTED=$(extract_json '.data.imported')
log "✓ Imported $IMPORTED function(s)"
print_last
rm -f "$EXPORT_FILE"
log ""

# --- Test 27: Test function with microapi.patch ---
log "TEST 27: Create function using microapi.patch"
http POST "$BASE/$SET/_functions" '{
  "id": "update_stock",
  "name": "Update Product Stock",
  "code": "local product = microapi.get(\"products\", input.product_id)\nif not product then\n  http_status = 404\n  output = {error = \"Product not found\"}\n  return\nend\n\nlocal updated = microapi.patch(\"products\", input.product_id, {\n  stock = input.new_stock\n})\n\nhttp_status = 200\noutput = {\n  product = updated,\n  message = \"Stock updated\"\n}",
  "timeout": 5000
}'
assert_status 201
log "✓ Patch function created"
log ""

# --- Test 28: Execute patch function ---
log "TEST 28: Execute patch function to update stock"
http POST "$BASE/$SET/_functions/update_stock" "{
  \"product_id\": \"$PRODUCT_ID\",
  \"new_stock\": 50
}"
assert_status 200
assert_json_equals '.data.product.stock' '50'
log "✓ Stock updated via patch"
log ""

# --- Test 29: Test function with microapi.delete ---
log "TEST 29: Create delete function"
http POST "$BASE/$SET/_functions" '{
  "id": "delete_product",
  "name": "Delete Product",
  "code": "local success = microapi.delete(\"products\", input.product_id)\nif not success then\n  http_status = 404\n  output = {error = \"Product not found or already deleted\"}\n  return\nend\n\nhttp_status = 200\noutput = {message = \"Product deleted\"}",
  "timeout": 5000
}'
assert_status 201
log "✓ Delete function created"
log ""

# --- Test 30: Execute delete function ---
log "TEST 30: Execute delete function"
http POST "$BASE/$SET/_functions/delete_product" "{
  \"product_id\": \"$PRODUCT_ID\"
}"
assert_status 200
assert_json_equals '.success' 'true'
log "✓ Product deleted via function"
log ""

# --- Test 31: Verify product was deleted ---
log "TEST 31: Verify product deletion"
http GET "$BASE/$SET/products/$PRODUCT_ID"
assert_status 404
log "✓ Product deletion confirmed"
log ""

# --- Test 32: Test execution statistics ---
log "TEST 32: Check function execution statistics"
http GET "$BASE/$SET/_functions/echo_func"
assert_status 200
TOTAL_EXECS=$(extract_json '.data.stats.total_executions')
if [[ "$TOTAL_EXECS" =~ ^[0-9]+$ ]] && [[ "$TOTAL_EXECS" -gt 0 ]]; then
  log "✓ Function has execution stats (total: $TOTAL_EXECS)"
else
  echo "ASSERT FAIL: Expected positive execution count" >&2
  exit 1
fi
print_last
log ""

# --- Test 33: Test function with multiple operations (shopping cart scenario) ---
log "TEST 33: Create shopping cart function"
http POST "$BASE/$SET/_functions" '{
  "id": "add_to_cart",
  "name": "Add to Cart",
  "code": "local product = microapi.get(\"products\", input.product_id)\nif not product then\n  http_status = 404\n  output = {error = \"Product not found\"}\n  return\nend\n\nlocal carts = microapi.query(\"carts\", {user_id = input.user_id})\nlocal cart\nif #carts == 0 then\n  cart = microapi.create(\"carts\", {\n    user_id = input.user_id,\n    items = {},\n    total = 0\n  })\nelse\n  cart = carts[1]\nend\n\ntable.insert(cart.items, {\n  product_id = input.product_id,\n  quantity = input.quantity or 1\n})\n\nlocal total = 0\nfor i, item in ipairs(cart.items) do\n  total = total + item.quantity\nend\ncart.total = total\n\ncart = microapi.update(\"carts\", cart._meta.id, cart)\n\nhttp_status = 200\noutput = {cart = cart}",
  "timeout": 5000
}'
assert_status 201
log "✓ Shopping cart function created"
log ""

# --- Test 34: Create test product for cart ---
log "TEST 34: Create test product for cart"
http POST "$BASE/$SET/products" '{
  "name": "Test Product",
  "price": 19.99,
  "stock": 10
}'
assert_status 201
CART_PRODUCT_ID=$(extract_json '.data._meta.id')
log "✓ Test product created: $CART_PRODUCT_ID"
log ""

# --- Test 35: Execute shopping cart function ---
log "TEST 35: Add product to cart"
http POST "$BASE/$SET/_functions/add_to_cart" "{
  \"user_id\": \"user_123\",
  \"product_id\": \"$CART_PRODUCT_ID\",
  \"quantity\": 2
}"
assert_status 200
assert_json_equals '.success' 'true'
CART_ITEMS=$(extract_json '.data.cart.items | length')
log "✓ Cart updated (items: $CART_ITEMS)"
log ""

# --- Test 36: Test invalid function ID ---
log "TEST 36: Test creating function with invalid ID"
http POST "$BASE/$SET/_functions" '{
  "id": "invalid-id-with-dashes",
  "name": "Invalid",
  "code": "http_status = 200"
}'
assert_status 400
assert_json_contains '.error' 'alphanumeric'
log "✓ Invalid function ID rejected"
log ""

# --- Test 37: Test function with no code ---
log "TEST 37: Test creating function without code"
http POST "$BASE/$SET/_functions" '{
  "id": "no_code",
  "name": "No Code"
}'
assert_status 400
assert_json_contains '.error' 'code is required'
log "✓ Missing code rejected"
log ""

# --- Test 38: Test timeout configuration ---
log "TEST 38: Create function with custom timeout"
http POST "$BASE/$SET/_functions" '{
  "id": "custom_timeout",
  "name": "Custom Timeout",
  "code": "http_status = 200\noutput = {timeout = \"custom\"}",
  "timeout": 10000
}'
assert_status 201
assert_json_equals '.data.timeout' '10000'
log "✓ Custom timeout accepted"
log ""

# --- Final Summary ---
log ""
log "========================================="
log "  All tests passed! ✓"
log "========================================="
log ""
log "Summary:"
log "  - Function CRUD operations: ✓"
log "  - Function execution: ✓"
log "  - Database operations (create, query, update, patch, delete): ✓"
log "  - Transaction rollback: ✓"
log "  - Sandbox mode: ✓"
log "  - Export/Import: ✓"
log "  - Input validation: ✓"
log "  - Error handling: ✓"
log "  - Execution statistics: ✓"
log "  - Complex multi-operation functions: ✓"
log ""
log "Lua Functions are working correctly!"
