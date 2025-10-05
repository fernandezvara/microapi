package luafn

import (
	"fmt"
	"time"
)

// Function represents a stored Lua function definition
type Function struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]any         `json:"input_schema,omitempty"`
	Code        string                 `json:"code"`
	Timeout     int                    `json:"timeout"` // milliseconds
	Stats       *FunctionStats         `json:"stats,omitempty"`
	Meta        *FunctionMeta          `json:"_meta,omitempty"`
}

// FunctionMeta holds metadata for a function
type FunctionMeta struct {
	ID        string `json:"id"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// FunctionStats holds execution statistics for a function
type FunctionStats struct {
	TotalExecutions int64              `json:"total_executions"`
	SuccessCount    int64              `json:"success_count"`
	ErrorCount      int64              `json:"error_count"`
	SuccessRate     float64            `json:"success_rate"`
	AvgDurationMs   float64            `json:"avg_duration_ms"`
	LastExecuted    string             `json:"last_executed,omitempty"`
	ErrorBreakdown  map[string]int64   `json:"error_breakdown,omitempty"`
}

// FunctionExecution represents a single execution request
type FunctionExecution struct {
	Input map[string]any `json:"input,omitempty"`
}

// FunctionExecutionResponse represents the response from a function execution
type FunctionExecutionResponse struct {
	Success bool           `json:"success"`
	Data    map[string]any `json:"data,omitempty"`
	Error   string         `json:"error,omitempty"`
	Message string         `json:"message,omitempty"`
	Meta    *ExecutionMeta `json:"_meta,omitempty"`
}

// ExecutionMeta holds metadata about the execution
type ExecutionMeta struct {
	ExecutionID string `json:"execution_id"`
	FunctionID  string `json:"function_id"`
	DurationMs  int64  `json:"duration_ms"`
	Timestamp   string `json:"timestamp"`
	Logs        []string `json:"logs,omitempty"`
}

// SandboxRequest represents a request to test a function in sandbox mode
type SandboxRequest struct {
	Code  string         `json:"code"`
	Input map[string]any `json:"input,omitempty"`
	Timeout int          `json:"timeout,omitempty"`
}

// SandboxResponse represents the response from sandbox execution
type SandboxResponse struct {
	Success    bool           `json:"success"`
	Data       *SandboxResult `json:"data,omitempty"`
	Error      string         `json:"error,omitempty"`
	Message    string         `json:"message,omitempty"`
}

// SandboxResult holds the result of a sandbox execution
type SandboxResult struct {
	HTTPStatus int            `json:"http_status"`
	Output     map[string]any `json:"output"`
	DurationMs int64          `json:"duration_ms"`
	Logs       []string       `json:"logs"`
	Warning    string         `json:"warning"`
}

// ExportRequest represents a request to export functions
type ExportRequest struct {
	FunctionIDs []string `json:"function_ids,omitempty"` // empty = all
}

// ExportResponse represents an export of one or more functions
type ExportResponse struct {
	Version    string      `json:"version"`
	ExportedAt string      `json:"exported_at"`
	Set        string      `json:"set,omitempty"`
	Function   *Function   `json:"function,omitempty"`   // for single export
	Functions  []*Function `json:"functions,omitempty"`  // for bulk export
}

// ImportRequest represents a request to import functions
type ImportRequest struct {
	Version   string      `json:"version"`
	Functions []*Function `json:"functions"`
	Options   *ImportOptions `json:"options,omitempty"`
}

// ImportOptions controls import behavior
type ImportOptions struct {
	Overwrite bool `json:"overwrite"` // overwrite existing functions
	Validate  bool `json:"validate"`  // validate code before importing
}

// ImportResponse represents the result of an import operation
type ImportResponse struct {
	Success  bool                  `json:"success"`
	Data     *ImportResult         `json:"data,omitempty"`
	Error    string                `json:"error,omitempty"`
}

// ImportResult holds the results of an import operation
type ImportResult struct {
	Imported int                    `json:"imported"`
	Skipped  int                    `json:"skipped"`
	Failed   int                    `json:"failed"`
	Details  []*ImportDetail        `json:"details"`
}

// ImportDetail holds the result for a single function import
type ImportDetail struct {
	ID     string `json:"id"`
	Status string `json:"status"` // "imported", "skipped", "failed"
	Reason string `json:"reason,omitempty"`
}

// NewFunctionStats creates a new empty stats object
func NewFunctionStats() *FunctionStats {
	return &FunctionStats{
		TotalExecutions: 0,
		SuccessCount:    0,
		ErrorCount:      0,
		SuccessRate:     0.0,
		AvgDurationMs:   0.0,
		ErrorBreakdown:  make(map[string]int64),
	}
}

// UpdateStats updates the function statistics after an execution
func (s *FunctionStats) UpdateStats(httpStatus int, duration time.Duration) {
	s.TotalExecutions++
	s.LastExecuted = time.Now().UTC().Format(time.RFC3339)

	// Update success/error counts
	if httpStatus >= 200 && httpStatus < 300 {
		s.SuccessCount++
	} else {
		s.ErrorCount++
	}

	// Update success rate
	if s.TotalExecutions > 0 {
		s.SuccessRate = float64(s.SuccessCount) / float64(s.TotalExecutions)
	}

	// Update average duration (simple rolling average)
	durationMs := float64(duration.Milliseconds())
	s.AvgDurationMs = ((s.AvgDurationMs * float64(s.TotalExecutions-1)) + durationMs) / float64(s.TotalExecutions)

	// Update error breakdown
	statusKey := fmt.Sprintf("%d", httpStatus)
	if s.ErrorBreakdown == nil {
		s.ErrorBreakdown = make(map[string]int64)
	}
	s.ErrorBreakdown[statusKey]++
}
