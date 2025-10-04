package luafn

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"microapi/internal/database"
)

const functionsCollection = "_functions"

// Storage handles persistence of Lua functions
type Storage struct {
	db *sql.DB
}

// NewStorage creates a new Storage instance
func NewStorage(db *sql.DB) *Storage {
	return &Storage{db: db}
}

// CreateFunction stores a new function in the database
func (s *Storage) CreateFunction(set string, fn *Function) error {
	// Ensure the set table exists
	if err := database.EnsureSetTable(s.db, set); err != nil {
		return err
	}

	// Ensure metadata for _functions collection
	if err := database.EnsureCollectionMetadata(s.db, set, functionsCollection); err != nil {
		return err
	}

	// Set default timeout if not specified
	if fn.Timeout == 0 {
		fn.Timeout = 5000 // 5 seconds default
	}

	// Validate timeout
	if fn.Timeout > 30000 {
		return fmt.Errorf("timeout cannot exceed 30000ms")
	}

	// Initialize stats
	if fn.Stats == nil {
		fn.Stats = NewFunctionStats()
	}

	// Build the data object (without _meta)
	data := map[string]any{
		"name":         fn.Name,
		"description":  fn.Description,
		"code":         fn.Code,
		"timeout":      fn.Timeout,
		"stats":        fn.Stats,
	}

	if fn.InputSchema != nil {
		data["input_schema"] = fn.InputSchema
	}

	dataBytes, err := json.Marshal(data)
	if err != nil {
		return err
	}

	now := time.Now().Unix()
	table := database.TableName(set)

	_, err = s.db.Exec(
		fmt.Sprintf("INSERT INTO %s (id, collection, data, created_at, updated_at) VALUES (?, ?, ?, ?, ?)", table),
		fn.ID, functionsCollection, string(dataBytes), now, now,
	)

	return err
}

// GetFunction retrieves a function by ID
func (s *Storage) GetFunction(set, id string) (*Function, error) {
	table := database.TableName(set)

	var dataStr string
	var created, updated int64
	err := s.db.QueryRow(
		fmt.Sprintf("SELECT data, created_at, updated_at FROM %s WHERE id = ? AND collection = ?", table),
		id, functionsCollection,
	).Scan(&dataStr, &created, &updated)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("function not found")
	}
	if err != nil {
		return nil, err
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(dataStr), &data); err != nil {
		return nil, err
	}

	fn := &Function{
		ID: id,
		Meta: &FunctionMeta{
			CreatedAt: created,
			UpdatedAt: updated,
		},
	}

	// Extract fields
	if v, ok := data["name"].(string); ok {
		fn.Name = v
	}
	if v, ok := data["description"].(string); ok {
		fn.Description = v
	}
	if v, ok := data["code"].(string); ok {
		fn.Code = v
	}
	if v, ok := data["timeout"].(float64); ok {
		fn.Timeout = int(v)
	}
	if v, ok := data["input_schema"].(map[string]any); ok {
		fn.InputSchema = v
	}
	if v, ok := data["stats"].(map[string]any); ok {
		fn.Stats = unmarshalStats(v)
	}

	return fn, nil
}

// ListFunctions returns all functions in a set
func (s *Storage) ListFunctions(set string) ([]*Function, error) {
	table := database.TableName(set)

	rows, err := s.db.Query(
		fmt.Sprintf("SELECT id, data, created_at, updated_at FROM %s WHERE collection = ?", table),
		functionsCollection,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var functions []*Function
	for rows.Next() {
		var id, dataStr string
		var created, updated int64
		if err := rows.Scan(&id, &dataStr, &created, &updated); err != nil {
			continue
		}

		var data map[string]any
		if err := json.Unmarshal([]byte(dataStr), &data); err != nil {
			continue
		}

		fn := &Function{
			ID: id,
			Meta: &FunctionMeta{
				CreatedAt: created,
				UpdatedAt: updated,
			},
		}

		// Extract fields
		if v, ok := data["name"].(string); ok {
			fn.Name = v
		}
		if v, ok := data["description"].(string); ok {
			fn.Description = v
		}
		if v, ok := data["code"].(string); ok {
			fn.Code = v
		}
		if v, ok := data["timeout"].(float64); ok {
			fn.Timeout = int(v)
		}
		if v, ok := data["input_schema"].(map[string]any); ok {
			fn.InputSchema = v
		}
		if v, ok := data["stats"].(map[string]any); ok {
			fn.Stats = unmarshalStats(v)
		}

		functions = append(functions, fn)
	}

	if functions == nil {
		functions = []*Function{}
	}

	return functions, nil
}

// UpdateFunction updates an existing function
func (s *Storage) UpdateFunction(set string, fn *Function) error {
	table := database.TableName(set)

	// Validate timeout
	if fn.Timeout > 30000 {
		return fmt.Errorf("timeout cannot exceed 30000ms")
	}

	// Build the data object
	data := map[string]any{
		"name":         fn.Name,
		"description":  fn.Description,
		"code":         fn.Code,
		"timeout":      fn.Timeout,
		"stats":        fn.Stats,
	}

	if fn.InputSchema != nil {
		data["input_schema"] = fn.InputSchema
	}

	dataBytes, err := json.Marshal(data)
	if err != nil {
		return err
	}

	now := time.Now().Unix()

	_, err = s.db.Exec(
		fmt.Sprintf("UPDATE %s SET data = ?, updated_at = ? WHERE id = ? AND collection = ?", table),
		string(dataBytes), now, fn.ID, functionsCollection,
	)

	return err
}

// DeleteFunction deletes a function by ID
func (s *Storage) DeleteFunction(set, id string) error {
	table := database.TableName(set)

	_, err := s.db.Exec(
		fmt.Sprintf("DELETE FROM %s WHERE id = ? AND collection = ?", table),
		id, functionsCollection,
	)

	return err
}

// UpdateFunctionStats updates only the stats for a function
func (s *Storage) UpdateFunctionStats(set, id string, stats *FunctionStats) error {
	// Get the current function
	fn, err := s.GetFunction(set, id)
	if err != nil {
		return err
	}

	// Update stats
	fn.Stats = stats

	// Save the function
	return s.UpdateFunction(set, fn)
}

// unmarshalStats converts a map to FunctionStats
func unmarshalStats(data map[string]any) *FunctionStats {
	stats := NewFunctionStats()

	if v, ok := data["total_executions"].(float64); ok {
		stats.TotalExecutions = int64(v)
	}
	if v, ok := data["success_count"].(float64); ok {
		stats.SuccessCount = int64(v)
	}
	if v, ok := data["error_count"].(float64); ok {
		stats.ErrorCount = int64(v)
	}
	if v, ok := data["success_rate"].(float64); ok {
		stats.SuccessRate = v
	}
	if v, ok := data["avg_duration_ms"].(float64); ok {
		stats.AvgDurationMs = v
	}
	if v, ok := data["last_executed"].(string); ok {
		stats.LastExecuted = v
	}
	if v, ok := data["error_breakdown"].(map[string]any); ok {
		stats.ErrorBreakdown = make(map[string]int64)
		for k, val := range v {
			if num, ok := val.(float64); ok {
				stats.ErrorBreakdown[k] = int64(num)
			}
		}
	}

	return stats
}
