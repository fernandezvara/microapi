package luafn_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "modernc.org/sqlite"

	"microapi/internal/config"
	"microapi/internal/database"
	"microapi/internal/server"
)

func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	// Run migrations
	if err := database.Migrate(db); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	return db
}

func TestFunctionCRUD(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	cfg := &config.Config{
		AllowDeleteCollections: true,
		MaxRequestSize:         10 * 1024 * 1024,
		CORSOrigins:            []string{"*"},
	}

	srv := server.New(cfg, db, "test")

	// Test 1: Create a function
	createReq := map[string]any{
		"id":          "test_func",
		"name":        "Test Function",
		"description": "A test function",
		"code": `
			log.info("Test function executed")
			http_status = 200
			output = {message = "Hello from Lua", input_value = input.value}
		`,
		"timeout": 5000,
	}

	body, _ := json.Marshal(createReq)
	req := httptest.NewRequest("POST", "/testset/_functions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var createResp map[string]any
	json.NewDecoder(w.Body).Decode(&createResp)
	if !createResp["success"].(bool) {
		t.Errorf("Expected success=true")
	}

	// Test 2: List functions
	req = httptest.NewRequest("GET", "/testset/_functions", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}

	var listResp map[string]any
	json.NewDecoder(w.Body).Decode(&listResp)
	data := listResp["data"].([]any)
	if len(data) != 1 {
		t.Errorf("Expected 1 function, got %d", len(data))
	}

	// Test 3: Get specific function
	req = httptest.NewRequest("GET", "/testset/_functions/test_func", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}

	// Test 4: Update function
	updateReq := map[string]any{
		"id":          "test_func",
		"name":        "Updated Test Function",
		"description": "An updated test function",
		"code": `
			http_status = 200
			output = {message = "Updated"}
		`,
		"timeout": 3000,
	}

	body, _ = json.Marshal(updateReq)
	req = httptest.NewRequest("PUT", "/testset/_functions/test_func", bytes.NewReader(body))
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Test 5: Delete function
	req = httptest.NewRequest("DELETE", "/testset/_functions/test_func", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}

	// Verify deletion
	req = httptest.NewRequest("GET", "/testset/_functions/test_func", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404 after deletion, got %d", w.Code)
	}
}

func TestFunctionExecution(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	cfg := &config.Config{
		AllowDeleteCollections: true,
		MaxRequestSize:         10 * 1024 * 1024,
		CORSOrigins:            []string{"*"},
	}

	srv := server.New(cfg, db, "test")

	// Create a function
	createReq := map[string]any{
		"id":          "echo_func",
		"name":        "Echo Function",
		"description": "Returns the input",
		"code": `
			http_status = 200
			output = {
				echoed = input.message,
				timestamp = ctx.timestamp
			}
		`,
		"timeout": 5000,
	}

	body, _ := json.Marshal(createReq)
	req := httptest.NewRequest("POST", "/testset/_functions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("Failed to create function: %d - %s", w.Code, w.Body.String())
	}

	// Execute the function
	execReq := map[string]any{
		"message": "Hello, World!",
	}

	body, _ = json.Marshal(execReq)
	req = httptest.NewRequest("POST", "/testset/_functions/echo_func", bytes.NewReader(body))
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var execResp map[string]any
	json.NewDecoder(w.Body).Decode(&execResp)

	if !execResp["success"].(bool) {
		t.Errorf("Expected success=true")
	}

	data := execResp["data"].(map[string]any)
	if data["echoed"] != "Hello, World!" {
		t.Errorf("Expected echoed message, got %v", data)
	}
}

func TestFunctionWithDatabaseOperations(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	cfg := &config.Config{
		AllowDeleteCollections: true,
		MaxRequestSize:         10 * 1024 * 1024,
		CORSOrigins:            []string{"*"},
	}

	srv := server.New(cfg, db, "test")

	// Create a function that performs database operations
	createReq := map[string]any{
		"id":          "db_func",
		"name":        "Database Function",
		"description": "Creates and queries documents",
		"code": `
			-- Create a product
			local product = microapi.create("products", {
				name = input.product_name,
				price = input.price,
				stock = input.stock
			})

			if not product then
				http_status = 500
				output = {error = "Failed to create product"}
				return
			end

			-- Query the product back
			local products = microapi.query("products", {name = input.product_name})

			http_status = 200
			output = {
				created_id = product._meta.id,
				found_count = #products,
				product = product
			}
		`,
		"timeout": 5000,
	}

	body, _ := json.Marshal(createReq)
	req := httptest.NewRequest("POST", "/testset/_functions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("Failed to create function: %d - %s", w.Code, w.Body.String())
	}

	// Execute the function
	execReq := map[string]any{
		"product_name": "Widget",
		"price":        29.99,
		"stock":        100,
	}

	body, _ = json.Marshal(execReq)
	req = httptest.NewRequest("POST", "/testset/_functions/db_func", bytes.NewReader(body))
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var execResp map[string]any
	json.NewDecoder(w.Body).Decode(&execResp)

	if !execResp["success"].(bool) {
		t.Errorf("Expected success=true, got: %v", execResp)
	}

	data := execResp["data"].(map[string]any)
	if data["found_count"].(float64) != 1 {
		t.Errorf("Expected to find 1 product, got %v", data["found_count"])
	}
}

func TestFunctionRollback(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	cfg := &config.Config{
		AllowDeleteCollections: true,
		MaxRequestSize:         10 * 1024 * 1024,
		CORSOrigins:            []string{"*"},
	}

	srv := server.New(cfg, db, "test")

	// Create a function that creates a document but returns an error
	createReq := map[string]any{
		"id":          "rollback_func",
		"name":        "Rollback Function",
		"description": "Tests transaction rollback",
		"code": `
			-- Create a product
			local product = microapi.create("products", {
				name = "ShouldRollback",
				price = 99.99
			})

			-- Return error status (should trigger rollback)
			http_status = 400
			output = {error = "Intentional error"}
		`,
		"timeout": 5000,
	}

	body, _ := json.Marshal(createReq)
	req := httptest.NewRequest("POST", "/testset/_functions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("Failed to create function: %d - %s", w.Code, w.Body.String())
	}

	// Execute the function
	req = httptest.NewRequest("POST", "/testset/_functions/rollback_func", bytes.NewReader([]byte("{}")))
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}

	// Verify that the product was NOT created (rollback worked)
	req = httptest.NewRequest("GET", `/testset/products?where={"name":{"$eq":"ShouldRollback"}}`, nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	// The collection should be empty or not exist (both are valid since rollback occurred)
	// If status is 200, verify empty array
	if w.Code == http.StatusOK {
		var queryResp map[string]any
		json.NewDecoder(w.Body).Decode(&queryResp)
		if queryResp["data"] != nil {
			data := queryResp["data"].([]any)
			if len(data) != 0 {
				t.Errorf("Expected product to be rolled back, but found %d products", len(data))
			}
		}
	}
}

func TestSandboxMode(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	cfg := &config.Config{
		AllowDeleteCollections: true,
		MaxRequestSize:         10 * 1024 * 1024,
		CORSOrigins:            []string{"*"},
	}

	srv := server.New(cfg, db, "test")

	// Test sandbox execution - simple test that doesn't require DB operations
	sandboxReq := map[string]any{
		"code": `
			http_status = 200
			output = {
				message = "Sandbox test",
				input_value = input.test
			}
		`,
		"input": map[string]any{
			"test": "value",
		},
		"timeout": 5000,
	}

	body, _ := json.Marshal(sandboxReq)
	req := httptest.NewRequest("POST", "/testset/_functions/_sandbox", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var sandboxResp map[string]any
	json.NewDecoder(w.Body).Decode(&sandboxResp)

	if !sandboxResp["success"].(bool) {
		t.Errorf("Expected success=true, got response: %v", sandboxResp)
	}

	// Verify data is returned
	data := sandboxResp["data"].(map[string]any)
	if data["warning"] != "Sandbox mode - no changes were saved" {
		t.Errorf("Expected sandbox warning")
	}
}

func TestExportImport(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	cfg := &config.Config{
		AllowDeleteCollections: true,
		MaxRequestSize:         10 * 1024 * 1024,
		CORSOrigins:            []string{"*"},
	}

	srv := server.New(cfg, db, "test")

	// Create a function
	createReq := map[string]any{
		"id":          "export_test",
		"name":        "Export Test Function",
		"description": "Test export/import",
		"code":        `http_status = 200; output = {ok = true}`,
		"timeout":     5000,
	}

	body, _ := json.Marshal(createReq)
	req := httptest.NewRequest("POST", "/testset/_functions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("Failed to create function: %d", w.Code)
	}

	// Export the function
	req = httptest.NewRequest("GET", "/testset/_functions/export_test?export=true", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}

	var exportResp map[string]any
	json.NewDecoder(w.Body).Decode(&exportResp)
	exportData := exportResp["data"].(map[string]any)

	if exportData["version"] != "1.0" {
		t.Errorf("Expected version 1.0")
	}

	// Delete the function
	req = httptest.NewRequest("DELETE", "/testset/_functions/export_test", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	// Import it back
	importReq := map[string]any{
		"version": "1.0",
		"functions": []map[string]any{
			{
				"id":          "export_test",
				"name":        "Export Test Function",
				"description": "Test export/import",
				"code":        `http_status = 200; output = {ok = true}`,
				"timeout":     5000,
			},
		},
		"options": map[string]any{
			"overwrite": true,
			"validate":  true,
		},
	}

	body, _ = json.Marshal(importReq)
	req = httptest.NewRequest("POST", "/testset/_functions/_import", bytes.NewReader(body))
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var importResp map[string]any
	json.NewDecoder(w.Body).Decode(&importResp)
	
	if !importResp["success"].(bool) {
		t.Errorf("Expected successful import")
	}

	// Verify the function exists again
	req = httptest.NewRequest("GET", "/testset/_functions/export_test", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected function to exist after import, got %d", w.Code)
	}
}
