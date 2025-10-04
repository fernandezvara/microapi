package luafn

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/xid"

	"microapi/internal/middleware"
	"microapi/internal/models"
)

// Handlers manages function-related HTTP handlers
type Handlers struct {
	db      *sql.DB
	storage *Storage
	service *Service
}

// NewHandlers creates a new Handlers instance
func NewHandlers(db *sql.DB) *Handlers {
	return &Handlers{
		db:      db,
		storage: NewStorage(db),
		service: NewService(),
	}
}

var functionIDRe = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// ValidateFunctionID checks if a function ID is valid
func ValidateFunctionID(id string) bool {
	return functionIDRe.MatchString(id)
}

// CreateFunction handles POST /{set}/_functions
func (h *Handlers) CreateFunction(w http.ResponseWriter, r *http.Request) {
	set := chi.URLParam(r, "set")

	var fn Function
	if err := json.NewDecoder(r.Body).Decode(&fn); err != nil {
		middleware.WriteJSON(w, http.StatusBadRequest, false, nil, models.Ptr("invalid JSON body"))
		return
	}

	// Validate required fields
	if fn.ID == "" {
		middleware.WriteJSON(w, http.StatusBadRequest, false, nil, models.Ptr("id is required"))
		return
	}
	if !ValidateFunctionID(fn.ID) {
		middleware.WriteJSON(w, http.StatusBadRequest, false, nil, models.Ptr("id must be alphanumeric with underscores only"))
		return
	}
	if fn.Code == "" {
		middleware.WriteJSON(w, http.StatusBadRequest, false, nil, models.Ptr("code is required"))
		return
	}

	// Set default timeout if not specified
	if fn.Timeout == 0 {
		fn.Timeout = 5000
	}

	// Validate code syntax
	if err := h.validateLuaCode(fn.Code); err != nil {
		middleware.WriteJSON(w, http.StatusBadRequest, false, nil, models.Ptr(fmt.Sprintf("code validation failed: %v", err)))
		return
	}

	// Check if function already exists
	existing, _ := h.storage.GetFunction(set, fn.ID)
	if existing != nil {
		middleware.WriteJSON(w, http.StatusConflict, false, nil, models.Ptr("function already exists"))
		return
	}

	// Initialize stats
	fn.Stats = NewFunctionStats()

	// Create the function
	if err := h.storage.CreateFunction(set, &fn); err != nil {
		middleware.WriteJSON(w, http.StatusInternalServerError, false, nil, models.Ptr(err.Error()))
		return
	}

	// Retrieve the created function to get metadata
	created, err := h.storage.GetFunction(set, fn.ID)
	if err != nil {
		middleware.WriteJSON(w, http.StatusInternalServerError, false, nil, models.Ptr(err.Error()))
		return
	}

	middleware.WriteJSON(w, http.StatusCreated, true, created, nil)
}

// ListFunctions handles GET /{set}/_functions
func (h *Handlers) ListFunctions(w http.ResponseWriter, r *http.Request) {
	set := chi.URLParam(r, "set")

	// Check for export query param
	if r.URL.Query().Get("export") == "true" {
		h.ExportAllFunctions(w, r)
		return
	}

	functions, err := h.storage.ListFunctions(set)
	if err != nil {
		middleware.WriteJSON(w, http.StatusInternalServerError, false, nil, models.Ptr(err.Error()))
		return
	}

	middleware.WriteJSON(w, http.StatusOK, true, functions, nil)
}

// GetFunction handles GET /{set}/_functions/{id}
func (h *Handlers) GetFunction(w http.ResponseWriter, r *http.Request) {
	set := chi.URLParam(r, "set")
	id := chi.URLParam(r, "id")

	// Check for export query param
	if r.URL.Query().Get("export") == "true" {
		h.ExportFunction(w, r)
		return
	}

	fn, err := h.storage.GetFunction(set, id)
	if err != nil {
		if err.Error() == "function not found" {
			middleware.WriteJSON(w, http.StatusNotFound, false, nil, models.Ptr("function not found"))
			return
		}
		middleware.WriteJSON(w, http.StatusInternalServerError, false, nil, models.Ptr(err.Error()))
		return
	}

	middleware.WriteJSON(w, http.StatusOK, true, fn, nil)
}

// UpdateFunction handles PUT /{set}/_functions/{id}
func (h *Handlers) UpdateFunction(w http.ResponseWriter, r *http.Request) {
	set := chi.URLParam(r, "set")
	id := chi.URLParam(r, "id")

	var fn Function
	if err := json.NewDecoder(r.Body).Decode(&fn); err != nil {
		middleware.WriteJSON(w, http.StatusBadRequest, false, nil, models.Ptr("invalid JSON body"))
		return
	}

	// Ensure ID matches
	fn.ID = id

	// Validate code if provided
	if fn.Code != "" {
		if err := h.validateLuaCode(fn.Code); err != nil {
			middleware.WriteJSON(w, http.StatusBadRequest, false, nil, models.Ptr(fmt.Sprintf("code validation failed: %v", err)))
			return
		}
	}

	// Get existing function to preserve stats
	existing, err := h.storage.GetFunction(set, id)
	if err != nil {
		if err.Error() == "function not found" {
			middleware.WriteJSON(w, http.StatusNotFound, false, nil, models.Ptr("function not found"))
			return
		}
		middleware.WriteJSON(w, http.StatusInternalServerError, false, nil, models.Ptr(err.Error()))
		return
	}

	// Preserve stats from existing function
	if fn.Stats == nil {
		fn.Stats = existing.Stats
	}

	// Set default timeout if not specified
	if fn.Timeout == 0 {
		fn.Timeout = 5000
	}

	// Update the function
	if err := h.storage.UpdateFunction(set, &fn); err != nil {
		middleware.WriteJSON(w, http.StatusInternalServerError, false, nil, models.Ptr(err.Error()))
		return
	}

	// Retrieve the updated function
	updated, err := h.storage.GetFunction(set, id)
	if err != nil {
		middleware.WriteJSON(w, http.StatusInternalServerError, false, nil, models.Ptr(err.Error()))
		return
	}

	middleware.WriteJSON(w, http.StatusOK, true, updated, nil)
}

// DeleteFunction handles DELETE /{set}/_functions/{id}
func (h *Handlers) DeleteFunction(w http.ResponseWriter, r *http.Request) {
	set := chi.URLParam(r, "set")
	id := chi.URLParam(r, "id")

	if err := h.storage.DeleteFunction(set, id); err != nil {
		middleware.WriteJSON(w, http.StatusInternalServerError, false, nil, models.Ptr(err.Error()))
		return
	}

	middleware.WriteJSON(w, http.StatusOK, true, map[string]any{"deleted": id}, nil)
}

// ExecuteFunction handles POST /{set}/_functions/{id}
func (h *Handlers) ExecuteFunction(w http.ResponseWriter, r *http.Request) {
	set := chi.URLParam(r, "set")
	id := chi.URLParam(r, "id")

	// Get the function
	fn, err := h.storage.GetFunction(set, id)
	if err != nil {
		if err.Error() == "function not found" {
			middleware.WriteJSON(w, http.StatusNotFound, false, nil, models.Ptr("function not found"))
			return
		}
		middleware.WriteJSON(w, http.StatusInternalServerError, false, nil, models.Ptr(err.Error()))
		return
	}

	// Parse input
	var input map[string]any
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			// Allow empty body
			input = make(map[string]any)
		}
	} else {
		input = make(map[string]any)
	}

	// Execute in a transaction
	tx, err := h.db.Begin()
	if err != nil {
		middleware.WriteJSON(w, http.StatusInternalServerError, false, nil, models.Ptr("failed to start transaction"))
		return
	}

	// Create execution context
	execID := xid.New().String()
	execCtx := &ExecutionContext{
		FunctionID:  fn.ID,
		ExecutionID: execID,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Set:         set,
		DB:          h.db,
		Tx:          tx,
		Logs:        []string{},
	}

	// Execute the function
	timeout := time.Duration(fn.Timeout) * time.Millisecond
	result := h.service.ExecuteFunction(context.Background(), execCtx, fn.Code, input, timeout)

	// Update stats
	if fn.Stats == nil {
		fn.Stats = NewFunctionStats()
	}
	fn.Stats.UpdateStats(result.HTTPStatus, result.Duration)

	// Determine whether to commit or rollback based on HTTP status
	shouldCommit := result.HTTPStatus >= 200 && result.HTTPStatus < 300 && result.Error == nil

	if shouldCommit {
		// Commit the transaction
		if err := tx.Commit(); err != nil {
			middleware.WriteJSON(w, http.StatusInternalServerError, false, nil, models.Ptr("failed to commit transaction"))
			return
		}
	} else {
		// Rollback the transaction
		tx.Rollback()
	}

	// Update function stats (in a separate transaction)
	go func() {
		h.storage.UpdateFunctionStats(set, fn.ID, fn.Stats)
	}()

	// Build response
	meta := &ExecutionMeta{
		ExecutionID: execID,
		FunctionID:  fn.ID,
		DurationMs:  result.Duration.Milliseconds(),
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Logs:        result.Logs,
	}

	if result.Error != nil {
		response := &FunctionExecutionResponse{
			Success: false,
			Error:   "Function execution failed",
			Message: result.Error.Error(),
			Meta:    meta,
		}
		middleware.WriteJSON(w, result.HTTPStatus, false, response.Data, models.Ptr(response.Error))
		return
	}

	response := &FunctionExecutionResponse{
		Success: true,
		Data:    result.Output,
		Meta:    meta,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(result.HTTPStatus)
	json.NewEncoder(w).Encode(response)
}

// ExecuteSandbox handles POST /{set}/_functions/_sandbox
func (h *Handlers) ExecuteSandbox(w http.ResponseWriter, r *http.Request) {
	set := chi.URLParam(r, "set")

	var req SandboxRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteJSON(w, http.StatusBadRequest, false, nil, models.Ptr("invalid JSON body"))
		return
	}

	if req.Code == "" {
		middleware.WriteJSON(w, http.StatusBadRequest, false, nil, models.Ptr("code is required"))
		return
	}

	// Set default timeout
	if req.Timeout == 0 {
		req.Timeout = 5000
	}

	// Validate code syntax
	if err := h.validateLuaCode(req.Code); err != nil {
		middleware.WriteJSON(w, http.StatusBadRequest, false, nil, models.Ptr(fmt.Sprintf("code validation failed: %v", err)))
		return
	}

	// Execute in a transaction that will always rollback
	tx, err := h.db.Begin()
	if err != nil {
		middleware.WriteJSON(w, http.StatusInternalServerError, false, nil, models.Ptr("failed to start transaction"))
		return
	}
	defer tx.Rollback() // Always rollback in sandbox mode

	// Create execution context
	execID := xid.New().String()
	execCtx := &ExecutionContext{
		FunctionID:  "_sandbox",
		ExecutionID: execID,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Set:         set,
		DB:          h.db,
		Tx:          tx,
		Logs:        []string{},
	}

	// Execute the function
	timeout := time.Duration(req.Timeout) * time.Millisecond
	result := h.service.ExecuteFunction(context.Background(), execCtx, req.Code, req.Input, timeout)

	// Build response - always return the sandbox result
	sandboxResult := &SandboxResult{
		HTTPStatus: result.HTTPStatus,
		Output:     result.Output,
		DurationMs: result.Duration.Milliseconds(),
		Logs:       result.Logs,
		Warning:    "Sandbox mode - no changes were saved",
	}

	response := &SandboxResponse{
		Success: result.Error == nil,
		Data:    sandboxResult,
	}

	if result.Error != nil {
		response.Error = "Sandbox execution failed"
		response.Message = result.Error.Error()
	}

	middleware.WriteJSON(w, http.StatusOK, response.Success, response.Data, func() *string {
		if result.Error != nil {
			return models.Ptr(response.Error)
		}
		return nil
	}())
}

// ExportFunction handles GET /{set}/_functions/{id}?export=true
func (h *Handlers) ExportFunction(w http.ResponseWriter, r *http.Request) {
	set := chi.URLParam(r, "set")
	id := chi.URLParam(r, "id")

	fn, err := h.storage.GetFunction(set, id)
	if err != nil {
		if err.Error() == "function not found" {
			middleware.WriteJSON(w, http.StatusNotFound, false, nil, models.Ptr("function not found"))
			return
		}
		middleware.WriteJSON(w, http.StatusInternalServerError, false, nil, models.Ptr(err.Error()))
		return
	}

	// Remove stats and meta for export
	fn.Stats = nil
	fn.Meta = nil

	export := &ExportResponse{
		Version:    "1.0",
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Function:   fn,
	}

	middleware.WriteJSON(w, http.StatusOK, true, export, nil)
}

// ExportAllFunctions handles GET /{set}/_functions?export=true
func (h *Handlers) ExportAllFunctions(w http.ResponseWriter, r *http.Request) {
	set := chi.URLParam(r, "set")

	functions, err := h.storage.ListFunctions(set)
	if err != nil {
		middleware.WriteJSON(w, http.StatusInternalServerError, false, nil, models.Ptr(err.Error()))
		return
	}

	// Remove stats and meta for export
	for _, fn := range functions {
		fn.Stats = nil
		fn.Meta = nil
	}

	export := &ExportResponse{
		Version:    "1.0",
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Set:        set,
		Functions:  functions,
	}

	middleware.WriteJSON(w, http.StatusOK, true, export, nil)
}

// ImportFunctions handles POST /{set}/_functions/_import
func (h *Handlers) ImportFunctions(w http.ResponseWriter, r *http.Request) {
	set := chi.URLParam(r, "set")

	var req ImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteJSON(w, http.StatusBadRequest, false, nil, models.Ptr("invalid JSON body"))
		return
	}

	if len(req.Functions) == 0 {
		middleware.WriteJSON(w, http.StatusBadRequest, false, nil, models.Ptr("no functions to import"))
		return
	}

	// Set default options
	if req.Options == nil {
		req.Options = &ImportOptions{
			Overwrite: false,
			Validate:  true,
		}
	}

	result := &ImportResult{
		Details: []*ImportDetail{},
	}

	for _, fn := range req.Functions {
		detail := &ImportDetail{
			ID:     fn.ID,
			Status: "imported",
		}

		// Validate function ID
		if !ValidateFunctionID(fn.ID) {
			detail.Status = "failed"
			detail.Reason = "invalid function ID"
			result.Failed++
			result.Details = append(result.Details, detail)
			continue
		}

		// Validate code if requested
		if req.Options.Validate && fn.Code != "" {
			if err := h.validateLuaCode(fn.Code); err != nil {
				detail.Status = "failed"
				detail.Reason = fmt.Sprintf("code validation failed: %v", err)
				result.Failed++
				result.Details = append(result.Details, detail)
				continue
			}
		}

		// Check if function exists
		existing, _ := h.storage.GetFunction(set, fn.ID)
		if existing != nil && !req.Options.Overwrite {
			detail.Status = "skipped"
			detail.Reason = "already exists"
			result.Skipped++
			result.Details = append(result.Details, detail)
			continue
		}

		// Initialize stats
		fn.Stats = NewFunctionStats()

		// Create or update function
		var err error
		if existing != nil {
			// Preserve existing stats
			fn.Stats = existing.Stats
			err = h.storage.UpdateFunction(set, fn)
		} else {
			err = h.storage.CreateFunction(set, fn)
		}

		if err != nil {
			detail.Status = "failed"
			detail.Reason = err.Error()
			result.Failed++
		} else {
			result.Imported++
		}

		result.Details = append(result.Details, detail)
	}

	response := &ImportResponse{
		Success: result.Failed == 0,
		Data:    result,
	}

	if response.Success {
		middleware.WriteJSON(w, http.StatusOK, true, response.Data, nil)
	} else {
		middleware.WriteJSON(w, http.StatusOK, false, response.Data, models.Ptr("some imports failed"))
	}
}

// validateLuaCode performs basic syntax validation on Lua code
func (h *Handlers) validateLuaCode(code string) error {
	// Check for dangerous patterns (basic security check)
	dangerous := []string{
		"require", "dofile", "loadfile", "load(",
	}

	lowerCode := strings.ToLower(code)
	for _, pattern := range dangerous {
		if strings.Contains(lowerCode, pattern) {
			return fmt.Errorf("code contains dangerous pattern: %s", pattern)
		}
	}

	// Try to compile the code (without executing)
	L := h.service.getVM()
	defer h.service.putVM(L)

	if _, err := L.LoadString(code); err != nil {
		return err
	}

	return nil
}
